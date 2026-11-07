package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type memoryStore struct {
	sessions map[[32]byte]Session
	err      error
}

func (s *memoryStore) Create(_ context.Context, h [32]byte, id string, expiry time.Time) error {
	if s.err != nil {
		return s.err
	}
	s.sessions[h] = Session{id, expiry}
	return nil
}
func (s *memoryStore) Lookup(_ context.Context, h [32]byte) (Session, error) {
	if s.err != nil {
		return Session{}, s.err
	}
	v, ok := s.sessions[h]
	if !ok || !time.Now().Before(v.ExpiresAt) {
		return Session{}, ErrUnauthenticated
	}
	return v, nil
}
func (s *memoryStore) Delete(_ context.Context, h [32]byte) error {
	if s.err != nil {
		return s.err
	}
	delete(s.sessions, h)
	return nil
}
func (s *memoryStore) DeleteUserSessions(_ context.Context, id string) error {
	for h, v := range s.sessions {
		if v.UserID == id {
			delete(s.sessions, h)
		}
	}
	return s.err
}

func TestSessionLifecycle(t *testing.T) {
	store := &memoryStore{sessions: make(map[[32]byte]Session)}
	m := NewManager(store, true)
	r := httptest.NewRequest("POST", "https://example.com/login", nil)
	w := httptest.NewRecorder()
	if err := m.Start(w, r, "user-1"); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	if cookie.Name != "__Host-session" || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(raw) != 32 {
		t.Fatal("invalid random token")
	}
	if _, ok := store.sessions[sha256.Sum256(raw)]; !ok {
		t.Fatal("hash not stored")
	}
	r = httptest.NewRequest("GET", "https://example.com/profile", nil)
	r.AddCookie(cookie)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserID(r.Context())
		if !ok || id != "user-1" {
			t.Error("missing user context")
		}
		w.WriteHeader(204)
	})
	w = httptest.NewRecorder()
	m.Middleware(next).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	rotated := httptest.NewRecorder()
	if err := m.Start(rotated, r, "user-1"); err != nil {
		t.Fatal(err)
	}
	if rotated.Result().Cookies()[0].Value == cookie.Value || len(store.sessions) != 1 {
		t.Fatal("session did not rotate")
	}
	w = httptest.NewRecorder()
	m.Middleware(next).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("old session still valid")
	}
	r = httptest.NewRequest("POST", "https://example.com/logout", nil)
	r.AddCookie(rotated.Result().Cookies()[0])
	w = httptest.NewRecorder()
	if err := m.End(w, r); err != nil {
		t.Fatal(err)
	}
	if len(store.sessions) != 0 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not revoke and clear")
	}
}

func TestMiddlewareAccess(t *testing.T) {
	store := &memoryStore{sessions: make(map[[32]byte]Session)}
	m := NewManager(store, false)
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/", 204}, {"HEAD", "/events", 204}, {"GET", "/login", 204}, {"POST", "/login", 204}, {"GET", "/healthz", 204}, {"GET", "/static/app.css", 204},
		{"GET", "/profile", 401}, {"GET", "/matches", 401}, {"GET", "/new-private-route", 401}, {"GET", "/events/private", 401}, {"POST", "/events", 401}, {"POST", "/logout", 401}, {"GET", "/api/me", 401}, {"POST", "/static/app.css", 401},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
	for _, token := range []string{"garbage", base64.RawURLEncoding.EncodeToString(make([]byte, 32))} {
		r := httptest.NewRequest("GET", "/profile", nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("invalid token accepted")
		}
	}
	raw := make([]byte, 32)
	hash := sha256.Sum256(raw)
	store.sessions[hash] = Session{"expired", time.Now().Add(-time.Second)}
	r := httptest.NewRequest("GET", "/profile", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: base64.RawURLEncoding.EncodeToString(raw)})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("expired token accepted")
	}
	store.err = errors.New("offline")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("database error failed open")
	}
	r = httptest.NewRequest("GET", "/profile", nil)
	r.Header.Set("HX-Request", "true")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 || w.Header().Get("HX-Redirect") != "" {
		t.Fatal("unexpected HTMX redirect")
	}
}
