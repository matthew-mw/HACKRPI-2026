package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/matthew-mw/HACKRPI-2026/internal/api"
)

type Credentials interface {
	Authenticate(context.Context, string, string) (string, error)
}

type PasswordCredentials struct{ DB *sql.DB }

// A valid cost-10 hash keeps unknown-user checks comparable to normal logins.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (p *PasswordCredentials) Authenticate(ctx context.Context, email, password string) (string, error) {
	var id string
	var hash sql.NullString
	err := p.DB.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE lower(email) = lower($1)`, strings.TrimSpace(email)).Scan(&id, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	stored := hash.String
	if !hash.Valid || stored == "" {
		stored = dummyHash
	}
	match := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password))
	if err != nil || !hash.Valid || hash.String == "" || match != nil || len(password) > 72 {
		return "", ErrUnauthenticated
	}
	return id, nil
}

type LoginHandler struct {
	Sessions    *Manager
	Credentials Credentials
	// Limits concurrent expensive bcrypt operations per server process.
	checks chan struct{}
}

func NewLoginHandler(sessions *Manager, credentials Credentials) *LoginHandler {
	return &LoginHandler{Sessions: sessions, Credentials: credentials, checks: make(chan struct{}, 4)}
}

func (h *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		api.Error(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid JSON login body")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		api.Error(w, http.StatusBadRequest, "expected a single JSON object")
		return
	}
	email, password := strings.TrimSpace(input.Email), input.Password
	if email == "" || len(email) > 254 || password == "" || len(password) > 72 {
		api.Error(w, http.StatusBadRequest, "email and password are required and must be within length limits")
		return
	}
	select {
	case h.checks <- struct{}{}:
		defer func() { <-h.checks }()
	default:
		w.Header().Set("Retry-After", "1")
		api.Error(w, http.StatusTooManyRequests, "Please try again shortly.")
		return
	}
	id, err := h.Credentials.Authenticate(r.Context(), email, password)
	if errors.Is(err, ErrUnauthenticated) {
		api.Error(w, http.StatusUnauthorized, "Invalid email or password.")
		return
	}
	if err != nil {
		api.Error(w, http.StatusServiceUnavailable, "authentication temporarily unavailable")
		return
	}
	if err := h.Sessions.Start(w, r, id); err != nil {
		api.Error(w, http.StatusServiceUnavailable, "authentication temporarily unavailable")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"user_id": id})
}

func (h *LoginHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.Sessions.End(w, r); err != nil {
		api.Error(w, http.StatusServiceUnavailable, "logout temporarily unavailable")
		return
	}
	api.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
