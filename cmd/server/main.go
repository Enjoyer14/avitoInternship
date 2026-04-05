package main

import (
	"log"
	"net/http"
	"os"

	"avito/internal/config"
	"avito/internal/httpapi"
	"avito/internal/store"
)

func main() {
	cfg := config.Load()

	s, err := store.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	defer s.Close()

	migrationDir := "migrations"
	if _, err := os.Stat(migrationDir); err != nil {
		migrationDir = "/app/migrations"
	}
	if err := s.RunMigrations(migrationDir); err != nil {
		log.Fatalf("migration error: %v", err)
	}

	h := &httpapi.Handler{Store: s, JWTSecret: cfg.JWTSecret}
	router := h.Router()

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	log.Printf("server started on :%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
