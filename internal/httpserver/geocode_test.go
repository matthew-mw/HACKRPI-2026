package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestAddressLookup(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		status, want int
	}{
		{"resolved", `{"result":{"addressMatches":[{"matchedAddress":"110 8TH ST, TROY, NY, 12180","coordinates":{"x":-73.68,"y":42.73},"addressComponents":{"city":"TROY","state":"NY","zip":"12180"}}]}}`, 200, 0},
		{"not found", `{"result":{"addressMatches":[]}}`, 200, 422},
		{"ambiguous", `{"result":{"addressMatches":[{},{}]}}`, 200, 422},
		{"malformed", `oops`, 200, 503},
		{"missing coordinates", `{"result":{"addressMatches":[{"matchedAddress":"TROY","addressComponents":{"city":"TROY"}}]}}`, 200, 503},
		{"provider unavailable", `{}`, 503, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("address") != "110 8th Street, Troy, NY 12180" || r.URL.Query().Get("benchmark") != "Public_AR_Current" || r.URL.Query().Get("format") != "json" {
					t.Error("wrong provider request")
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			got, err := lookupAddress(context.Background(), server.Client(), server.URL, "110 8th Street, Troy, NY 12180")
			if tc.want == 0 {
				if err != nil || got.City != "TROY" || got.Region != "NY" || got.PostalCode != "12180" || got.Latitude != 42.73 || got.Longitude != -73.68 {
					t.Fatalf("unexpected resolution: %+v %v", got, err)
				}
			} else {
				var pe *productError
				if !errors.As(err, &pe) || pe.status != tc.want {
					t.Fatalf("expected %d; got %v", tc.want, err)
				}
			}
		})
	}
}
func TestAddressLookupCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := lookupAddress(ctx, http.DefaultClient, "https://example.invalid", "110 8th Street, Troy, NY 12180")
	var pe *productError
	if !errors.As(err, &pe) || pe.status != 503 {
		t.Fatalf("cancelled lookup: %v", err)
	}
}

func TestAreaLookup(t *testing.T) {
	original := geocoderClient
	defer func() {
		geocoderClient = original
		areaLookupCache = map[string]map[string]any{}
		areaLookupLast = time.Time{}
	}()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("q") != "12180" || r.URL.Query().Get("countrycodes") != "us" {
			t.Error("incorrect area search")
		}
		w.Write([]byte(`[{"display_name":"Troy, New York, USA","lat":"42.73","lon":"-73.69"}]`))
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	geocoderClient = &http.Client{Transport: areaTestTransport{target: target}}
	areaLookupCache = map[string]map[string]any{}
	areaLookupLast = time.Time{}
	first, err := lookupArea(context.Background(), "12180")
	if err != nil {
		t.Fatal(err)
	}
	loc := first.(map[string]any)
	if loc["latitude"] != 42.73 || loc["city"] != "12180" {
		t.Fatalf("wrong area: %v", loc)
	}
	if _, err = lookupArea(context.Background(), "12180"); err != nil || calls != 1 {
		t.Fatalf("cache missed: %d %v", calls, err)
	}
	if _, err = lookupArea(context.Background(), ""); err == nil {
		t.Fatal("accepted empty location")
	}
}

type areaTestTransport struct{ target *url.URL }

func (a areaTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.Scheme = a.target.Scheme
	r.URL.Host = a.target.Host
	return http.DefaultTransport.RoundTrip(r)
}
