// Package httpserver serves the application pages, assets, and JSON endpoints.
package httpserver

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/matthew-mw/HACKRPI-2026/internal/api"
	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
	"github.com/matthew-mw/HACKRPI-2026/web/static"
	"github.com/matthew-mw/HACKRPI-2026/web/templates"
)

// NewHandler builds the application routes with session and cross-origin protection.
func NewHandler(sessions *auth.Manager, credentials auth.Credentials) http.Handler {
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
