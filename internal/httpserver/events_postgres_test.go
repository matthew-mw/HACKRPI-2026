package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Uses connection-local temporary tables; requires PostGIS in a disposable DB.
func TestPostgresEventDistanceAndTime(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL/PostGIS database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.ExecContext(ctx, `
CREATE TEMP TABLE locations (id uuid, label text, city text, coordinates geography(Point,4326),address_line1 text);
CREATE TEMP TABLE users (id uuid, location_id uuid, max_distance_km integer);
CREATE TEMP TABLE organizations (id uuid, name text, visibility text, owner_id uuid);
CREATE TEMP TABLE organization_members (organization_id uuid, user_id uuid, status text);
CREATE TEMP TABLE events (id uuid, organization_id uuid, location_id uuid, title text, description text, starts_at timestamptz, ends_at timestamptz, capacity integer, status text, visibility text DEFAULT 'public',created_by uuid,completed_at timestamptz);
INSERT INTO locations(id,label,city,coordinates) VALUES
 ('00000000-0000-0000-0000-000000000001','Home','Troy',ST_SetSRID(ST_MakePoint(-73.69,42.73),4326)::geography),
 ('00000000-0000-0000-0000-000000000002','Nearby','Troy',ST_SetSRID(ST_MakePoint(-73.69,42.74),4326)::geography),
 ('00000000-0000-0000-0000-000000000003','Far','Elsewhere',ST_SetSRID(ST_MakePoint(-73.69,43.73),4326)::geography);
INSERT INTO users VALUES ('00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000001',10);
INSERT INTO organizations VALUES ('00000000-0000-0000-0000-000000000001','Test organization','public',NULL);
INSERT INTO events(id,organization_id,location_id,title,description,starts_at,ends_at,capacity,status)
SELECT ('00000000-0000-0000-0000-00000000000'||n)::uuid,
 '00000000-0000-0000-0000-000000000001'::uuid,
 CASE WHEN n=3 THEN '00000000-0000-0000-0000-000000000003'::uuid ELSE '00000000-0000-0000-0000-000000000002'::uuid END,
 'Event '||n, '', now()+n*interval '1 day', now()+n*interval '1 day'+interval '1 hour', 10,
 CASE WHEN n=4 THEN 'draft' ELSE 'published' END
FROM generate_series(1,4) AS n;`)
	if err != nil {
		t.Fatal(err)
	}
	var serverNow time.Time
	if err = conn.QueryRowContext(ctx, "SELECT now()").Scan(&serverNow); err != nil {
		t.Fatal(err)
	}
	user := "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct {
		name                     string
		user, from, to, distance any
		count                    int
		first                    string
	}{
		{"chronological public events", nil, nil, nil, nil, 3, "Event 1"},
		{"profile distance excludes far event", user, nil, nil, nil, 2, "Event 1"},
		{"narrower requested radius", user, nil, nil, 1, 0, ""},
		{"requested radius cannot expand profile", user, nil, nil, 500, 2, "Event 1"},
		{"time window", nil, serverNow.Add(36 * time.Hour), serverNow.Add(60 * time.Hour), nil, 1, "Event 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			if err := conn.QueryRowContext(ctx, eventQuery, tc.user, tc.from, tc.to, nil, tc.distance).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var rows []struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(raw, &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.count {
				t.Fatalf("got %s; expected %d events", raw, tc.count)
			}
			if len(rows) > 0 && rows[0].Title != tc.first {
				t.Fatalf("unexpected first event: %s", raw)
			}
		})
	}
}
