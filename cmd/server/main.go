package main

import (
	"context"
	"log"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/auth"
	"github.com/bethmaloney/mailgun-mock-api/internal/config"
	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/server"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	if cfg.AuthMode == "disabled" {
		log.Println("WARNING: Auth is disabled — test data is unprotected. Set AUTH_MODE=entra for deployed instances.")
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	var validator *auth.Validator
	if cfg.AuthMode == "entra" {
		var err error
		validator, err = auth.NewValidator(ctx, cfg.EntraTenantID, "api://"+cfg.EntraClientID, cfg.EntraAPIScope)
		if err != nil {
			log.Fatalf("Failed to initialize auth validator: %v", err)
		}
	}

	handler := server.New(ctx, db, cfg, validator)

	log.Printf("Starting server on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
