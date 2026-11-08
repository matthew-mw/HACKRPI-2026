package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type resolvedAddress struct {
	Address, City, Region, PostalCode string
	Latitude, Longitude               float64
}

const censusGeocoder = "https://geocoding.geo.census.gov/geocoder/locations/onelineaddress"

var geocoderClient = &http.Client{Timeout: 7 * time.Second}

// Coordinates stay an internal implementation detail; never guess a failed lookup.
func lookupAddress(ctx context.Context, client *http.Client, endpoint, address string) (resolvedAddress, error) {
	var result resolvedAddress
	address = strings.TrimSpace(address)
	if len(address) < 8 || len(address) > 300 {
		return result, invalid("Enter a full street address, including city, state, and ZIP code.")
	}
	params := url.Values{"address": {address}, "benchmark": {"Public_AR_Current"}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	unavailable := &productError{503, "Address lookup is temporarily unavailable. Please try again in a moment."}
	if err != nil {
		return result, unavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, unavailable
	}
	var body struct {
		Result struct {
			Matches []struct {
				MatchedAddress string `json:"matchedAddress"`
				Coordinates    struct {
					X *float64 `json:"x"`
					Y *float64 `json:"y"`
				} `json:"coordinates"`
				Components struct {
					City  string `json:"city"`
					State string `json:"state"`
					Zip   string `json:"zip"`
				} `json:"addressComponents"`
			} `json:"addressMatches"`
		} `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body) != nil {
		return result, unavailable
	}
	if len(body.Result.Matches) != 1 {
		return result, &productError{422, "We couldn’t find one exact address. Check the street number, city, state, and ZIP code."}
	}
	match := body.Result.Matches[0]
	if match.Coordinates.X == nil || match.Coordinates.Y == nil || match.Components.City == "" || match.MatchedAddress == "" || *match.Coordinates.Y < -90 || *match.Coordinates.Y > 90 || *match.Coordinates.X < -180 || *match.Coordinates.X > 180 {
		return result, unavailable
	}
	return resolvedAddress{Address: match.MatchedAddress, City: match.Components.City, Region: match.Components.State, PostalCode: match.Components.Zip, Latitude: *match.Coordinates.Y, Longitude: *match.Coordinates.X}, nil
}

func (p product) resolveAddress(ctx context.Context, address string) (resolvedAddress, error) {
	// Reuse a known address before contacting the provider again.
	var result resolvedAddress
	err := p.db.QueryRowContext(ctx, `SELECT address_line1,city,COALESCE(region,''),COALESCE(postal_code,''),ST_Y(coordinates::geometry),ST_X(coordinates::geometry) FROM locations WHERE lower(address_line1)=lower($1) ORDER BY created_at DESC LIMIT 1`, strings.TrimSpace(address)).Scan(&result.Address, &result.City, &result.Region, &result.PostalCode, &result.Latitude, &result.Longitude)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	return lookupAddress(ctx, geocoderClient, censusGeocoder, address)
}

var areaLookupMu sync.Mutex
var areaLookupLast time.Time
var areaLookupCache = map[string]map[string]any{}

// lookupArea resolves a broad area without requiring a volunteer street address.
func lookupArea(ctx context.Context, query string) (any, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 || len(query) > 120 {
		return nil, invalid("Enter a city and state or ZIP code.")
	}
	areaLookupMu.Lock()
	defer areaLookupMu.Unlock()
	key := strings.ToLower(query)
	if cached, ok := areaLookupCache[key]; ok {
		return cached, nil
	}
	if wait := time.Until(areaLookupLast.Add(time.Second)); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	areaLookupLast = time.Now()
	params := url.Values{"q": {query}, "countrycodes": {"us"}, "format": {"jsonv2"}, "limit": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://nominatim.openstreetmap.org/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VolunteerTogether/1.0 (volunteer area lookup)")
	response, err := geocoderClient.Do(req)
	if err != nil {
		return nil, &productError{503, "Location search is unavailable. Please try again shortly."}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &productError{503, "Location search is unavailable. Please try again shortly."}
	}
	var rows []struct {
		Name      string `json:"display_name"`
		Latitude  string `json:"lat"`
		Longitude string `json:"lon"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&rows) != nil {
		return nil, &productError{503, "Location search is unavailable. Please try again shortly."}
	}
	if len(rows) == 0 {
		return nil, invalid("We could not find that area. Try a city and state or ZIP code.")
	}
	lat, e1 := strconv.ParseFloat(rows[0].Latitude, 64)
	lon, e2 := strconv.ParseFloat(rows[0].Longitude, 64)
	if e1 != nil || e2 != nil {
		return nil, invalid("Try a different city or ZIP code.")
	}
	if len(areaLookupCache) >= 1000 {
		areaLookupCache = map[string]map[string]any{}
	}
	result := map[string]any{"city": query, "label": rows[0].Name, "latitude": lat, "longitude": lon}
	areaLookupCache[key] = result
	return result, nil
}
