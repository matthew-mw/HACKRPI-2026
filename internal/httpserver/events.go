package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/matthew-mw/HACKRPI-2026/internal/api"
	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
)

// eventQuery deliberately uses hard filters only: there is no similarity ranking.
const eventQuery = `SELECT COALESCE(jsonb_agg(result ORDER BY starts_at, id), '[]'::jsonb)
FROM (
 SELECT e.id, e.organization_id, e.location_id, e.title, e.description,
        e.starts_at, e.ends_at, e.capacity, e.status,
        o.name AS organization_name, l.label AS location_label, l.city, l.address_line1 AS address,
        CASE WHEN $1::uuid IS NOT NULL THEN ST_Distance(l.coordinates, ul.coordinates) / 1000.0 END AS distance_km
 FROM events e
 JOIN locations l ON l.id = e.location_id
 JOIN organizations o ON o.id = e.organization_id
 LEFT JOIN users u ON u.id = $1::uuid
 LEFT JOIN locations ul ON ul.id = u.location_id
 WHERE e.status = 'published' AND e.starts_at >= now() AND e.completed_at IS NULL
   AND (e.visibility='public' OR o.owner_id=$1 OR e.created_by=$1 OR EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=o.id AND m.user_id=$1 AND m.status='active'))
   AND ($1::uuid IS NULL OR ST_DWithin(l.coordinates, ul.coordinates,
       LEAST(u.max_distance_km, COALESCE($5::integer, u.max_distance_km)) * 1000.0))
   AND ($2::timestamptz IS NULL OR e.starts_at >= $2)
   AND ($3::timestamptz IS NULL OR e.ends_at <= $3)
   AND ($4::integer IS NULL OR EXTRACT(DOW FROM e.starts_at AT TIME ZONE 'America/New_York') = $4)
 ORDER BY e.starts_at, e.id
) AS result`

func eventHandler(db *sql.DB, matches bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var from, to any
		var fromTime, toTime time.Time
		for _, key := range []string{"from", "to"} {
			if value := r.URL.Query().Get(key); value != "" {
				t, err := time.Parse(time.RFC3339, value)
				if err != nil {
					api.Error(w, 400, key+" must be an RFC3339 timestamp")
					return
				}
				if key == "from" {
					from, fromTime = t, t
				} else {
					to, toTime = t, t
				}
			}
		}
		if !fromTime.IsZero() && !toTime.IsZero() && !toTime.After(fromTime) {
			api.Error(w, 400, "to must be after from")
			return
		}
		var weekday, distance any
		for _, key := range []string{"weekday", "max_distance_km"} {
			if value := r.URL.Query().Get(key); value != "" {
				n, err := strconv.Atoi(value)
				if err != nil || (key == "weekday" && (n < 0 || n > 6)) || (key == "max_distance_km" && (n < 1 || n > 500)) {
					api.Error(w, 400, "invalid "+key)
					return
				}
				if key == "weekday" {
					weekday = n
				} else {
					distance = n
				}
			}
		}
		if db == nil {
			api.Error(w, http.StatusServiceUnavailable, "event database unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var user any
		if matches {
			id, ok := auth.UserID(ctx)
			if !ok {
				api.Error(w, 401, "authentication required")
				return
			}
			var hasLocation bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users u JOIN locations l ON l.id = u.location_id WHERE u.id = $1)`, id).Scan(&hasLocation); err != nil {
				api.Error(w, 503, "event database unavailable")
				return
			}
			if !hasLocation {
				api.Error(w, 422, "set your profile location before finding nearby matches")
				return
			}
			user = id
		}
		var result []byte
		if err := db.QueryRowContext(ctx, eventQuery, user, from, to, weekday, distance).Scan(&result); err != nil {
			api.Error(w, 503, "events temporarily unavailable")
			return
		}
		api.JSON(w, 200, json.RawMessage(result))
	}
}
