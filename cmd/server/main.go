package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.ParseFiles("templates/index.html")
		if err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		if err := t.Execute(w, nil); err != nil {
			http.Error(w, "render error", http.StatusInternalServerError)
		}
	})

	return mux
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: newMux(),
	}

	log.Printf("server listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
