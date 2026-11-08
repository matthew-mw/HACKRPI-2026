package httpserver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
)

type eventDriver struct{}
type eventConn struct{}
type eventRows struct{ done bool }

func (eventDriver) Open(string) (driver.Conn, error)  { return eventConn{}, nil }
func (eventConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (eventConn) Close() error                        { return nil }
func (eventConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (eventConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return &eventRows{}, nil
}
func (*eventRows) Columns() []string { return []string{"events"} }
func (*eventRows) Close() error      { return nil }
func (r *eventRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = []byte(`[]`)
	return nil
}

func init() { sql.Register("volunteer-events-test", eventDriver{}) }

func TestEventFiltersAndEmptyResults(t *testing.T) {
	db, err := sql.Open("volunteer-events-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := eventHandler(db, false)
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"", 200},
		{"?from=2026-11-14T09:00:00Z&to=2026-11-15T09:00:00Z&weekday=6", 200},
		{"?weekday=7", 400}, {"?weekday=-1", 400}, {"?weekday=oops", 400},
		{"?max_distance_km=0", 400}, {"?max_distance_km=501", 400},
		{"?from=tomorrow", 400}, {"?to=2026-11-15", 400},
		{"?from=2026-11-15T09:00:00Z&to=2026-11-14T09:00:00Z", 400},
	} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", "/events"+tc.query, nil))
		assertJSON(t, w, tc.status)
		if tc.status == 200 && strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatal("empty results must be an array")
		}
	}
}

func TestMatchesRequireSession(t *testing.T) {
	h := NewHandler(auth.NewManager(make(testStore), false), testCredentials{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/matches?weekday=6", nil))
	assertJSON(t, w, 401)
}
