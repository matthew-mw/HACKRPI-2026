package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/matthew-mw/HACKRPI-2026/internal/api"
)

type Registration struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type Registrar interface {
	Register(context.Context, Registration) (string, error)
}

var ErrEmailExists = errors.New("email already registered")

func (p *PasswordCredentials) Register(ctx context.Context, input Registration) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	var id string
	err = p.DB.QueryRowContext(ctx, `INSERT INTO users (first_name, last_name, email, password_hash) VALUES ($1,$2,$3,$4) RETURNING id`, input.FirstName, input.LastName, input.Email, string(hash)).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_users_email_ci" {
		return "", ErrEmailExists
	}
	return id, err
}

func (h *LoginHandler) Signup(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		api.Error(w, 415, "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var input Registration
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		api.Error(w, 400, "Invalid signup details.")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		api.Error(w, 400, "Expected a single JSON object.")
		return
	}
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 {
		api.Error(w, 400, "Enter a valid email address.")
		return
	}
	if input.FirstName == "" || input.LastName == "" || len(input.FirstName) > 100 || len(input.LastName) > 100 {
		api.Error(w, 400, "First and last names are required (up to 100 bytes each).")
		return
	}
	if utf8.RuneCountInString(input.Password) < 12 || len(input.Password) > 72 {
		api.Error(w, 400, "Choose at least 12 characters. If your password is too long, use a shorter phrase.")
		return
	}
	registrar, ok := h.Credentials.(Registrar)
	if !ok {
		api.Error(w, 503, "Signup temporarily unavailable.")
		return
	}
	select {
	case h.checks <- struct{}{}:
		defer func() { <-h.checks }()
	default:
		w.Header().Set("Retry-After", "1")
		api.Error(w, 429, "Please try again shortly.")
		return
	}
	id, err := registrar.Register(r.Context(), input)
	if errors.Is(err, ErrEmailExists) {
		api.Error(w, 409, "An account with this email already exists. Sign in instead.")
		return
	}
	if err != nil {
		api.Error(w, 503, "Signup temporarily unavailable.")
		return
	}
	if err := h.Sessions.Start(w, r, id); err != nil {
		api.Error(w, 503, "Your account was created. Please sign in to continue.")
		return
	}
	api.JSON(w, 201, map[string]string{"user_id": id})
}
