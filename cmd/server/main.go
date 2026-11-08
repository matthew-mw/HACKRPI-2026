package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/matthew-mw/HACKRPI-2026/internal/auth"
	"github.com/matthew-mw/HACKRPI-2026/internal/httpserver"
)

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
		Handler:           httpserver.NewHandler(sessions, &auth.PasswordCredentials{DB: database}, database),
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
