package main

import (
	"context"
	"database/sql"
	"github.com/matthew-mw/HACKRPI-2026/internal/api"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/matthew-mw/HACKRPI-2026/static"
	"github.com/matthew-mw/HACKRPI-2026/templates"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
)

// Exact routing avoids ServeMux's HTML redirects and plaintext 404/405 responses.
func newMux(sessions *auth.Manager, credentials auth.Credentials) http.Handler {
	login := auth.NewLoginHandler(sessions, credentials)
	pages := http.FileServerFS(templates.Files)
	assets := http.StripPrefix("/static/", http.FileServerFS(static.Files))
	pageNames := map[string]string{"/": "index.html", "/events": "events.html", "/profile": "profile.html", "/matches": "matches.html"}
	type route struct {
		method  string
		handler http.HandlerFunc
	}
	routes := map[string]route{
		"/": {"GET", func(w http.ResponseWriter, r *http.Request) {
			api.JSON(w, 200, map[string]string{"service": "CharityMatch"})
		}},
		"/healthz": {"GET", func(w http.ResponseWriter, r *http.Request) { api.JSON(w, 200, map[string]string{"status": "ok"}) }},
		"/events": {"GET", func(w http.ResponseWriter, r *http.Request) {
			api.Error(w, http.StatusNotImplemented, "events API is not implemented")
		}},
		"/matches": {"GET", func(w http.ResponseWriter, r *http.Request) {
			api.Error(w, http.StatusNotImplemented, "matches API is not implemented")
		}},
		"/profile": {"GET", func(w http.ResponseWriter, r *http.Request) {
			id, _ := auth.UserID(r.Context())
			api.JSON(w, 200, map[string]string{"user_id": id})
		}},
		"/login":  {"POST", login.Login},
		"/logout": {"POST", login.Logout},
		"/signup": {"POST", login.Signup},
	}
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == "GET" || r.Method == "HEAD") && (r.URL.Path == "/login" || r.URL.Path == "/signup") {
			pageRequest := r.Clone(r.Context())
			pageRequest.URL.Path += ".html"
			pages.ServeHTTP(w, pageRequest)
			return
		}
		if (r.Method == "GET" || r.Method == "HEAD") && strings.HasPrefix(r.URL.Path, "/static/") {
			name := strings.TrimPrefix(r.URL.Path, "/static/")
			info, err := fs.Stat(static.Files, name)
			if err != nil || info.IsDir() {
				api.Error(w, http.StatusNotFound, "asset not found")
				return
			}
			assets.ServeHTTP(w, r)
			return
		}
		route, ok := routes[r.URL.Path]
		if !ok {
			api.Error(w, http.StatusNotFound, "route not found")
			return
		}
		if r.Method != route.method && !(r.Method == "HEAD" && route.method == "GET") {
			allow := route.method
			if r.URL.Path == "/login" || r.URL.Path == "/signup" {
				allow = "GET, HEAD, POST"
			}
			if allow == "GET" {
				allow += ", HEAD"
			}
			w.Header().Set("Allow", allow)
			api.Error(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Add("Vary", "Accept")
		if name, ok := pageNames[r.URL.Path]; ok && strings.Contains(r.Header.Get("Accept"), "text/html") {
			// Serve a cloned request so authentication keeps the original route.
			pageRequest := r.Clone(r.Context())
			pageRequest.URL.Path = "/" + name
			if name == "index.html" {
				pageRequest.URL.Path = "/"
			}
			pages.ServeHTTP(w, pageRequest)
			return
		}
		route.handler(w, r)
	})
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.Error(w, http.StatusForbidden, "cross-origin request rejected")
	}))
	return protection.Handler(sessions.Middleware(router))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal("invalid database configuration")
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = database.PingContext(ctx)
	cancel()
	if err != nil {
		log.Fatal("database connection failed")
	}
	store := &auth.PostgresStore{DB: database}
	// Secure cookies are the default; opt out only for local HTTP development.
	sessions := auth.NewManager(store, os.Getenv("AUTH_INSECURE_LOCAL_HTTP") != "true")
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := store.DeleteExpired(ctx); err != nil {
				log.Print("expired session cleanup failed")
			}
			cancel()
		}
	}()
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           newMux(sessions, &auth.PasswordCredentials{DB: database}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("server listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
