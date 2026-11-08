package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
)

// Every record lives in an isolated schema, even when run against the test deployment.
func TestProductPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the full PostgreSQL product workflow")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("product_test_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema + ",public"
	db := stdlib.OpenDB(*config)
	defer db.Close()
	for _, name := range []string{"001_init.sql", "002_sessions.sql", "003_social_product.sql", "004_event_lifecycle.sql", "005_onboarding_charities.sql"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, string(b)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	handler := NewHandler(auth.NewManager(&auth.PostgresStore{DB: db}, false), &auth.PasswordCredentials{DB: db}, db)
	call := func(method, path string, body any, cookie *http.Cookie, want int) map[string]any {
		t.Helper()
		var encoded string
		if body != nil {
			b, _ := json.Marshal(body)
			encoded = string(b)
		}
		r := httptest.NewRequest(method, "http://example.com"+path, strings.NewReader(encoded))
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://example.com")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("not JSON object: %s", w.Body.String())
		}
		return out
	}
	signup := func(first string) (string, *http.Cookie) {
		t.Helper()
		body := fmt.Sprintf(`{"first_name":%q,"last_name":"Integration","email":%q,"password":"integration-test-password"}`, first, strings.ToLower(first)+"@example.com")
		r := httptest.NewRequest("POST", "http://example.com/signup", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
		var out map[string]string
		json.Unmarshal(w.Body.Bytes(), &out)
		return out["user_id"], w.Result().Cookies()[0]
	}
	owner, ownerCookie := signup("Organizer")
	volunteer, volCookie := signup("Volunteer")
	other, otherCookie := signup("Other")
	arrayCall := func(path string, cookie *http.Cookie, want int) []map[string]any {
		t.Helper()
		r := httptest.NewRequest("GET", "http://example.com"+path, nil)
		r.Header.Set("Accept", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("GET %s: got %d want %d: %s", path, w.Code, want, w.Body.String())
		}
		if want != 200 {
			return nil
		}
		var rows []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatalf("GET %s: %v: %s", path, err, w.Body.String())
		}
		return rows
	}
	baseProfile := func(first, visibility, hours string) map[string]any {
		return map[string]any{"first_name": first, "last_name": "Integration", "bio": "Actual profile", "max_distance_km": 10, "activity_visibility": visibility, "hours_visibility": hours, "volunteer_with_friends": true, "location": map[string]any{"label": "Starting point", "city": "Troy", "latitude": 42.73, "longitude": -73.69}, "tags": []any{map[string]any{"name": "Environment", "category": "cause"}}, "availability": []any{map[string]any{"weekday": int(time.Now().Add(24 * time.Hour).In(mustLocation()).Weekday()), "start_time": "00:00", "end_time": "23:59", "timezone": "America/New_York"}}}
	}
	call("POST", "/api/profile", baseProfile("Organizer", "private", "private"), ownerCookie, 200)
	call("POST", "/api/profile", baseProfile("Volunteer", "private", "private"), volCookie, 200)
	const address = "110 8TH ST, TROY, NY, 12180"
	if _, err = db.ExecContext(ctx, `UPDATE locations SET address_line1=$2,region='NY',postal_code='12180' WHERE id=(SELECT location_id FROM users WHERE id=$1)`, owner, address); err != nil {
		t.Fatal(err)
	}
	addressProfile := baseProfile("Volunteer", "private", "private")
	addressProfile["location"] = map[string]any{"address": address, "label": "Campus"}
	call("POST", "/api/profile", addressProfile, volCookie, 200)
	if call("GET", "/api/dashboard", nil, volCookie, 200)["profile"].(map[string]any)["address"] != address {
		t.Fatal("profile street address not saved")
	}
	organization := call("POST", "/api/organization", map[string]any{"name": "Integration crew", "description": "Isolated workflow test", "organization_type": "volunteer_group", "visibility": "public"}, ownerCookie, 200)["id"].(string)
	start := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	end := start.Add(time.Hour)
	event := call("POST", "/api/event", map[string]any{"organization_id": organization, "title": "Isolated test event", "description": "Not a production event", "starts_at": start, "ends_at": end, "capacity": 1, "label": "Test location", "address": address, "causes": []string{"Environment"}}, ownerCookie, 200)["id"].(string)
	call("POST", "/api/event", map[string]any{"organization_id": organization, "title": "Unauthorized event", "starts_at": start, "ends_at": end, "capacity": 1, "city": "Troy", "latitude": 42.74, "longitude": -73.69}, volCookie, 403)
	call("POST", "/api/registration", map[string]any{"event_id": event, "join": true}, volCookie, 200)
	call("POST", "/api/registration", map[string]any{"event_id": event, "join": true}, otherCookie, 409)
	call("POST", "/api/save", map[string]any{"event_id": event, "save": true}, volCookie, 200)
	rows := arrayCall("/api/opportunities?near=true&cause=Environment", volCookie, 200)
	if len(rows) != 1 || !rows[0]["joined"].(bool) || !rows[0]["saved"].(bool) || rows[0]["spots"].(float64) != 0 {
		t.Fatalf("opportunity state incorrect: %v", rows)
	}
	if rows[0]["address"] != address {
		t.Fatal("event address must appear in opportunity cards")
	}
	if len(arrayCall("/api/opportunities?q=no-such-event", volCookie, 200)) != 0 {
		t.Fatal("search must filter events")
	}
	arrayCall("/api/opportunities?near=true", nil, 401)
	arrayCall("/api/opportunities?near=true", otherCookie, 422)
	if len(arrayCall("/api/people", volCookie, 200)) != 2 {
		t.Fatal("people list must contain real accounts")
	}
	call("POST", "/api/membership", map[string]any{"organization_id": organization, "join": true}, volCookie, 200)
	call("POST", "/api/invite", map[string]any{"organization_id": organization, "user_ids": []string{other}}, ownerCookie, 200)
	invitations := call("GET", "/api/dashboard", nil, otherCookie, 200)["invitations"].(map[string]any)
	if len(invitations["crews"].([]any)) != 1 {
		t.Fatal("crew invite missing")
	}
	call("POST", "/api/membership", map[string]any{"organization_id": organization, "join": true}, otherCookie, 200)
	call("POST", "/api/challenge", map[string]any{"key": "ten-hours", "join": true}, volCookie, 200)
	dash := call("GET", "/api/dashboard", nil, volCookie, 200)
	if len(dash["upcoming"].([]any)) != 1 {
		t.Fatal("registration missing from upcoming")
	}
	// Group matching excludes members without a location/availability.
	if _, err = db.ExecContext(ctx, `UPDATE events SET capacity=3 WHERE id=$1`, event); err != nil {
		t.Fatal(err)
	}
	crew := call("GET", "/api/crew?id="+organization, nil, ownerCookie, 200)
	if len(crew["matches"].([]any)) != 0 {
		t.Fatal("unconfigured member must prevent group match")
	}
	call("POST", "/api/profile", baseProfile("Other", "private", "private"), otherCookie, 200)
	crew = call("GET", "/api/crew?id="+organization, nil, ownerCookie, 200)
	if len(crew["matches"].([]any)) != 1 {
		t.Fatal("configured crew should find its event")
	}
	// Advance only the isolated fixture, then log completed service through the API.
	start = time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	end = start.Add(time.Hour)
	if _, err = db.ExecContext(ctx, `UPDATE events SET starts_at=$2,ends_at=$3 WHERE id=$1`, event, start, end); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/activity", map[string]any{"event_id": event, "started_at": start, "ended_at": end, "caption": "Completed service"}, volCookie, 200)
	call("POST", "/api/activity", map[string]any{"event_id": event, "started_at": start, "ended_at": end}, volCookie, 409)
	mine := call("GET", "/api/dashboard", nil, volCookie, 200)
	stats := mine["stats"].(map[string]any)
	if stats["hours"].(float64) != 1 || stats["verified_hours"].(float64) != 0 {
		t.Fatalf("wrong pending totals: %v", stats)
	}
	activity := mine["my_activity"].([]any)[0].(map[string]any)["id"].(string)
	if len(call("GET", "/api/dashboard", nil, ownerCookie, 200)["feed"].([]any)) != 0 {
		t.Fatal("private activity leaked")
	}
	call("POST", "/api/reaction", map[string]any{"activity_id": activity, "like": true}, ownerCookie, 404)
	call("POST", "/api/profile", baseProfile("Volunteer", "public", "private"), volCookie, 200)
	shared := call("GET", "/api/dashboard", nil, ownerCookie, 200)["feed"].([]any)
	if len(shared) != 1 || shared[0].(map[string]any)["hours"] != nil {
		t.Fatal("hour privacy leaked")
	}
	call("POST", "/api/reaction", map[string]any{"activity_id": activity, "like": true}, ownerCookie, 200)
	call("POST", "/api/comment", map[string]any{"activity_id": activity, "body": "Thanks for showing up"}, ownerCookie, 200)
	call("POST", "/api/verify", map[string]any{"activity_id": activity, "status": "verified"}, volCookie, 409)
	if len(arrayCall("/api/review", ownerCookie, 200)) != 1 {
		t.Fatal("organizer review missing pending service")
	}
	call("POST", "/api/verify", map[string]any{"activity_id": activity, "status": "verified"}, ownerCookie, 200)
	if len(arrayCall("/api/leaderboard?period=all", ownerCookie, 200)) != 0 {
		t.Fatal("private service hours leaked in rankings")
	}
	if len(arrayCall("/api/leaderboard?period=all", volCookie, 200)) != 1 {
		t.Fatal("own verified hours should appear")
	}
	if len(arrayCall("/api/leaderboard?period=all&type=crews", ownerCookie, 200)) != 1 {
		t.Fatal("crew verified hours should appear")
	}
	if len(arrayCall("/api/leaderboard?period=all&type=organizations", ownerCookie, 200)) != 0 {
		t.Fatal("campus ranking must not contain volunteer crews")
	}
	mine = call("GET", "/api/dashboard", nil, volCookie, 200)
	if mine["stats"].(map[string]any)["verified_hours"].(float64) != 1 {
		t.Fatal("verification did not update totals")
	}
	own := mine["my_activity"].([]any)[0].(map[string]any)
	if own["kudos"].(float64) != 1 || len(own["comments"].([]any)) != 1 {
		t.Fatal("social state not persisted")
	}
	call("POST", "/api/profile", baseProfile("Volunteer", "friends", "friends"), volCookie, 200)
	call("POST", "/api/follow", map[string]any{"user_id": volunteer, "follow": true}, ownerCookie, 200)
	if len(call("GET", "/api/dashboard", nil, ownerCookie, 200)["feed"].([]any)) != 0 {
		t.Fatal("one-way follow must not unlock friends-only activity")
	}
	call("POST", "/api/follow", map[string]any{"user_id": owner, "follow": true}, volCookie, 200)
	shared = call("GET", "/api/dashboard", nil, ownerCookie, 200)["feed"].([]any)
	if len(shared) != 1 || shared[0].(map[string]any)["hours"].(float64) != 1 {
		t.Fatal("mutual friends should see shared hours")
	}
	if len(call("GET", "/api/dashboard", nil, otherCookie, 200)["feed"].([]any)) != 0 {
		t.Fatal("friends-only activity leaked to unrelated user")
	}
	public := call("GET", "/api/community", nil, nil, 200)
	if len(public["feed"].([]any)) != 0 {
		t.Fatal("private feed leaked to anonymous user")
	}
	call("POST", "/api/registration", map[string]any{"event_id": event, "join": false}, volCookie, 200)
	call("POST", "/api/membership", map[string]any{"organization_id": organization, "join": false}, ownerCookie, 409)
	// Event audiences are independent of the hosting crew's public membership.
	outsider, outsiderCookie := signup("Outsider")
	privateEvent := call("POST", "/api/event", map[string]any{"organization_id": organization, "title": "Crew only service", "description": "Private attendance workflow", "starts_at": time.Now().Add(24 * time.Hour), "ends_at": time.Now().Add(25 * time.Hour), "capacity": 5, "address": address, "visibility": "crew_only"}, ownerCookie, 200)["id"].(string)
	if len(arrayCall("/events", nil, 200)) != 0 {
		t.Fatal("crew-only event leaked through public events API")
	}
	if len(arrayCall("/api/opportunities", outsiderCookie, 200)) != 0 {
		t.Fatal("crew-only event leaked through discovery")
	}
	call("GET", "/api/event-details?id="+privateEvent, nil, outsiderCookie, 404)
	call("POST", "/api/registration", map[string]any{"event_id": privateEvent, "join": true}, outsiderCookie, 403)
	call("POST", "/api/invite", map[string]any{"event_id": privateEvent, "user_ids": []string{outsider}}, ownerCookie, 400)
	if len(call("GET", "/api/crew?id="+organization, nil, outsiderCookie, 200)["events"].([]any)) != 0 {
		t.Fatal("private event leaked through public crew preview")
	}
	call("POST", "/api/registration", map[string]any{"event_id": privateEvent, "join": true}, volCookie, 200)
	call("POST", "/api/registration", map[string]any{"event_id": privateEvent, "join": true}, otherCookie, 200)
	call("GET", "/api/event-attendance?id="+privateEvent, nil, volCookie, 403)
	if len(arrayCall("/api/my-events", ownerCookie, 200)) != 2 {
		t.Fatal("hosted event history missing")
	}
	call("POST", "/api/event-complete", map[string]any{"event_id": privateEvent, "participant_ids": []string{volunteer}, "started_at": time.Now(), "ended_at": time.Now().Add(time.Hour)}, ownerCookie, 400)
	actualStart := time.Now().Add(-5 * time.Hour).UTC().Truncate(time.Second)
	actualEnd := actualStart.Add(time.Hour)
	if _, err = db.ExecContext(ctx, `UPDATE events SET starts_at=$2,ends_at=$3 WHERE id=$1`, privateEvent, actualStart, actualEnd); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/activity", map[string]any{"event_id": privateEvent, "started_at": actualStart, "ended_at": actualEnd, "caption": "Earlier individual submission"}, volCookie, 200)
	completion := map[string]any{"event_id": privateEvent, "participant_ids": []string{volunteer, owner}, "started_at": actualStart, "ended_at": actualEnd}
	call("POST", "/api/event-complete", completion, outsiderCookie, 403)
	badParticipants := map[string]any{"event_id": privateEvent, "participant_ids": []string{outsider}, "started_at": actualStart, "ended_at": actualEnd}
	call("POST", "/api/event-complete", badParticipants, ownerCookie, 400)
	finished := call("POST", "/api/event-complete", completion, ownerCookie, 200)
	if finished["participants_credited"].(float64) != 2 {
		t.Fatal("confirmed participant not credited")
	}
	if call("POST", "/api/event-complete", completion, ownerCookie, 200)["already_completed"] != true {
		t.Fatal("repeated completion must be idempotent")
	}
	volStats := call("GET", "/api/dashboard", nil, volCookie, 200)["stats"].(map[string]any)
	if volStats["hours"].(float64) != 2 || volStats["verified_hours"].(float64) != 2 {
		t.Fatalf("completion double-counted or failed to verify hours: %v", volStats)
	}
	if call("GET", "/api/dashboard", nil, otherCookie, 200)["stats"].(map[string]any)["hours"].(float64) != 0 {
		t.Fatal("no-show received service hours")
	}
	var generated, superseded int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE completion_generated),count(*) FILTER(WHERE superseded_by_completion) FROM activities WHERE event_id=$1`, privateEvent).Scan(&generated, &superseded); err != nil || generated != 2 || superseded != 1 {
		t.Fatalf("completion audit records: %d %d %v", generated, superseded, err)
	}
	call("POST", "/api/registration", map[string]any{"event_id": privateEvent, "join": false}, volCookie, 409)
	call("POST", "/api/activity", map[string]any{"event_id": privateEvent, "started_at": actualStart, "ended_at": actualEnd}, volCookie, 409)
	if call("GET", "/api/dashboard", nil, ownerCookie, 200)["stats"].(map[string]any)["verified_hours"].(float64) != 1 {
		t.Fatal("participating organizer was not credited")
	}
	if call("GET", "/api/event-details?id="+privateEvent, nil, ownerCookie, 200)["completed_at"] == nil {
		t.Fatal("event was not explicitly marked completed")
	}

	charity := call("POST", "/api/organization", map[string]any{"name": "Review charity", "organization_type": "charity", "visibility": "public", "address": address}, ownerCookie, 200)["id"].(string)
	charityEvent := map[string]any{"organization_id": charity, "title": "Charity service", "starts_at": time.Now().Add(48 * time.Hour), "ends_at": time.Now().Add(49 * time.Hour), "capacity": 10, "address": address}
	call("POST", "/api/event", charityEvent, ownerCookie, 403)
	call("POST", "/api/charity-approval", map[string]any{"organization_id": charity}, ownerCookie, 403)
	if _, err = db.ExecContext(ctx, `UPDATE users SET is_site_admin=true WHERE id=$1`, owner); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/charity-approval", map[string]any{"organization_id": charity}, ownerCookie, 200)
	call("POST", "/api/membership", map[string]any{"organization_id": charity, "join": true}, volCookie, 200)
	call("POST", "/api/event", charityEvent, volCookie, 200)
	clubEvent := map[string]any{"organization_id": organization, "title": "Member cannot publish", "starts_at": time.Now().Add(48 * time.Hour), "ends_at": time.Now().Add(49 * time.Hour), "capacity": 10, "address": address}
	call("POST", "/api/event", clubEvent, volCookie, 403)
	t.Log("Signup, profile, publish, capacity, save, membership, invitation, group matching, activity, privacy, kudos, comments, verification and mutual follows passed")
}
func mustLocation() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return loc
}
