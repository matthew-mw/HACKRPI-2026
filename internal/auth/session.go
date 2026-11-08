// Package auth provides opaque, database-backed browser sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/matthew-mw/HACKRPI-2026/internal/api"
)

var ErrUnauthenticated = errors.New("unauthenticated")

type Session struct {
	UserID    string
	ExpiresAt time.Time
}

// Store implementations must enforce expiry in Lookup and never persist raw tokens.
type Store interface {
	Create(context.Context, [32]byte, string, time.Time) error
	Lookup(context.Context, [32]byte) (Session, error)
	Delete(context.Context, [32]byte) error
	DeleteUserSessions(context.Context, string) error
}

type Manager struct {
	store  Store
	secure bool
	ttl    time.Duration
}

func NewManager(store Store, secure bool) *Manager {
	return &Manager{store: store, secure: secure, ttl: 24 * time.Hour}
}

func (m *Manager) cookieName() string {
	if m.secure {
		return "__Host-session"
	}
	return "session" // Explicit local HTTP development mode only.
}

func (m *Manager) cookie(token string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{Name: m.cookieName(), Value: token, Path: "/", HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: maxAge}
}

func (m *Manager) tokenHash(r *http.Request) ([32]byte, error) {
	c, err := r.Cookie(m.cookieName())
	if err != nil {
		return [32]byte{}, ErrUnauthenticated
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(raw) != 32 {
		return [32]byte{}, ErrUnauthenticated
	}
	return sha256.Sum256(raw), nil
}

// Start rotates any existing browser session after successful authentication.
func (m *Manager) Start(w http.ResponseWriter, r *http.Request, userID string) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	if hash, err := m.tokenHash(r); err == nil {
		if err := m.store.Delete(r.Context(), hash); err != nil {
			return err
		}
	}
	expires := time.Now().Add(m.ttl)
	if err := m.store.Create(r.Context(), sha256.Sum256(raw), userID, expires); err != nil {
		return err
	}
	http.SetCookie(w, m.cookie(base64.RawURLEncoding.EncodeToString(raw), expires, int(m.ttl.Seconds())))
	return nil
}

func (m *Manager) End(w http.ResponseWriter, r *http.Request) error {
	if hash, err := m.tokenHash(r); err == nil {
		if err := m.store.Delete(r.Context(), hash); err != nil {
			return err
		}
	}
	http.SetCookie(w, m.cookie("", time.Unix(1, 0), -1))
	return nil
}

type userKey struct{}

func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userKey{}).(string)
	return id, ok && id != ""
}

// PublicRequest is intentionally method-aware and exact.
// Every new route is private unless explicitly added here.
func PublicRequest(r *http.Request) bool {
	if r.Method == http.MethodPost {
		return r.URL.Path == "/login" || r.URL.Path == "/signup"
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/static/") {
		return true
	}
	switch r.URL.Path {
	case "/", "/discover", "/events", "/healthz", "/login", "/signup", "/api/community", "/api/opportunities", "/api/event-details":
		return true
	}
	return false
}

func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep personalized responses and login cookies out of shared caches.
		w.Header().Set("Cache-Control", "no-store")
		if hash, err := m.tokenHash(r); err == nil {
			session, err := m.store.Lookup(r.Context(), hash)
			switch {
			case err == nil && session.UserID != "" && time.Now().Before(session.ExpiresAt):
				r = r.WithContext(context.WithValue(r.Context(), userKey{}, session.UserID))
			case err != nil && !errors.Is(err, ErrUnauthenticated):
				api.Error(w, http.StatusServiceUnavailable, "authentication temporarily unavailable")
				return
			default:
				http.SetCookie(w, m.cookie("", time.Unix(1, 0), -1))
			}
		}
		if _, ok := UserID(r.Context()); !ok && !PublicRequest(r) {
			if (r.Method == "GET" || r.Method == "HEAD") && isPrivatePage(r.URL.Path) && strings.Contains(r.Header.Get("Accept"), "text/html") {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			api.Error(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPrivatePage(path string) bool {
	switch path {
	case "/profile", "/matches", "/my-events", "/clubs", "/charities", "/challenges", "/impact":
		return true
	}
	return false
}
