package main

import (
	"log"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/config"
	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/logging"
	"github.com/bethmaloney/mailgun-mock-api/internal/server"
)

func main() {
	// Route slog + the legacy log package through an async handler so
	// no caller can block on a slow stderr pipe. Must run before any
	// HTTP traffic starts. See internal/logging for the rationale.
	logging.Init()

	cfg := config.Load()

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	_ = db

	handler := server.New(db)

	log.Printf("Starting server on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
