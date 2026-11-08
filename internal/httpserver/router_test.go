package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
)

type testStore map[[32]byte]auth.Session

func (s testStore) Create(_ context.Context, h [32]byte, id string, expiry time.Time) error {
	s[h] = auth.Session{UserID: id, ExpiresAt: expiry}
	return nil
}
func (s testStore) Lookup(_ context.Context, h [32]byte) (auth.Session, error) {
	v, ok := s[h]
	if !ok {
		return v, auth.ErrUnauthenticated
	}
	return v, nil
}
func (s testStore) Delete(_ context.Context, h [32]byte) error { delete(s, h); return nil }
func (s testStore) DeleteUserSessions(_ context.Context, id string) error {
	for h, v := range s {
		if v.UserID == id {
			delete(s, h)
		}
	}
	return nil
}

type testCredentials struct{}

func (testCredentials) Authenticate(_ context.Context, email, password string) (string, error) {
	if email == "user@example.com" && password == "correct" {
		return "user-1", nil
	}
	return "", auth.ErrUnauthenticated
}

func assertJSON(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("not JSON: %s", w.Body.String())
	}
	if w.Header().Get("Location") != "" || w.Header().Get("HX-Redirect") != "" {
		t.Fatal("unexpected redirect")
	}
}

func TestBrowserPagesAndAssets(t *testing.T) {
	handler := NewHandler(auth.NewManager(make(testStore), false), testCredentials{})
	for _, tc := range []struct{ path, content string }{
		{"/", "<h1>CharityMatch</h1>"},
		{"/events", "<h1>Events</h1>"},
		{"/login", "Welcome back"},
		{"/signup", "Create your account"},
		{"/static/css/site.css", "font-family"},
	} {
		for _, method := range []string{"GET", "HEAD"} {
			r := httptest.NewRequest(method, tc.path, nil)
			r.Header.Set("Accept", "text/html")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("%s %s: %d %s", method, tc.path, w.Code, w.Body.String())
			}
			if method == "GET" && !strings.Contains(w.Body.String(), tc.content) {
				t.Fatalf("missing content at %s", tc.path)
			}
			if method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
		}
	}
	for _, path := range []string{"/profile", "/matches", "/templates/profile.html", "/.env"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if path == "/profile" || path == "/matches" {
			if w.Code != 303 || w.Header().Get("Location") != "/login" {
				t.Fatal("missing login redirect")
			}
		} else {
			assertJSON(t, w, 401)
		}
	}
}

func TestRoutesAndLogin(t *testing.T) {
	store := make(testStore)
	handler := NewHandler(auth.NewManager(store, false), testCredentials{})
	request := func(method, path, body, contentType, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://example.com"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/", 200}, {"/events", 501}, {"/healthz", 200}, {"/profile", 401}, {"/matches", 401}, {"/future", 401}, {"/static/app.css", 404}} {
		assertJSON(t, request("GET", tc.path, "", "", "", nil), tc.status)
	}
	for _, tc := range []struct {
		body, contentType string
		status            int
	}{
		{`{"email":"user@example.com","password":"wrong"}`, "application/json", 401},
		{`{"email":"user@example.com","password":"correct"}`, "text/plain", 415},
		{`email=user@example.com&password=correct`, "application/x-www-form-urlencoded", 415},
		{`{`, "application/json", 400},
		{`null`, "application/json", 400},
		{`{}`, "application/json", 400},
		{`{"email":123}`, "application/json", 400},
		{`{"email":"user@example.com","password":"correct","next":"/"}`, "application/json", 400},
		{`{"email":"user@example.com","password":"correct"}{}`, "application/json", 400},
		{`{"email":"` + strings.Repeat("x", 9000) + `"}`, "application/json", 400},
	} {
		w := request("POST", "/login", tc.body, tc.contentType, "http://example.com", nil)
		assertJSON(t, w, tc.status)
		if len(w.Result().Cookies()) != 0 || len(store) != 0 {
			t.Fatal("invalid login created a session")
		}
	}
	body := `{"email":"user@example.com","password":"correct"}`
	assertJSON(t, request("POST", "/login", body, "application/json", "https://evil.test", nil), 403)
	w := request("POST", "/login", body, "application/json; charset=utf-8", "http://example.com", nil)
	assertJSON(t, w, 200)
	if !strings.Contains(w.Body.String(), `"user_id":"user-1"`) {
		t.Fatal("missing user ID")
	}
	cookie := w.Result().Cookies()[0]
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/profile", 200}, {"GET", "/matches", 501}, {"GET", "/missing", 404}, {"GET", "/static/app.css", 404}, {"PUT", "/profile", 405}, {"GET", "//profile", 404}, {"GET", "/events/../profile", 404}} {
		assertJSON(t, request(tc.method, tc.path, "", "", "", cookie), tc.status)
	}
	assertJSON(t, request("POST", "/logout", "", "", "https://evil.test", cookie), 403)
	if len(store) != 1 {
		t.Fatal("CSRF revoked session")
	}
	w = request("POST", "/logout", "", "", "http://example.com", cookie)
	assertJSON(t, w, 200)
	if len(store) != 0 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not revoke and clear")
	}
	assertJSON(t, request("GET", "/profile", "", "", "", cookie), 401)
}

func (testCredentials) Register(_ context.Context, input auth.Registration) (string, error) {
	return "user-1", nil
}

func TestSignupSession(t *testing.T) {
	store := make(testStore)
	handler := NewHandler(auth.NewManager(store, false), testCredentials{})
	body := `{"first_name":"Test","last_name":"User","email":"test@example.com","password":"long-password"}`
	for _, origin := range []string{"https://evil.test", "http://example.com"} {
		r := httptest.NewRequest("POST", "http://example.com/signup", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if origin == "https://evil.test" {
			assertJSON(t, w, 403)
			if len(store) != 0 {
				t.Fatal("CSRF created a session")
			}
			continue
		}
		assertJSON(t, w, 201)
		if len(store) != 1 || len(w.Result().Cookies()) != 1 {
			t.Fatal("missing signup session")
		}
		r = httptest.NewRequest("GET", "http://example.com/profile", nil)
		r.Header.Set("Accept", "text/html")
		r.AddCookie(w.Result().Cookies()[0])
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<h1>Profile</h1>") {
			t.Fatal("signup session cannot view profile")
		}
	}
}
