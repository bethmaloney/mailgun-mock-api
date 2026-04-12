package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port             string
	DatabaseURL      string
	DBDriver         string
	AuthMode         string
	EntraTenantID    string
	EntraClientID    string
	EntraAPIScope    string
	EntraRedirectURI string
}

func Load() *Config {
	return &Config{
		Port:             getEnv("PORT", "8025"),
		DatabaseURL:      getEnv("DATABASE_URL", "file:mailgun-mock.db"),
		DBDriver:         getEnv("DB_DRIVER", "sqlite"),
		AuthMode:         getEnv("AUTH_MODE", "disabled"),
		EntraTenantID:    getEnv("ENTRA_TENANT_ID", ""),
		EntraClientID:    getEnv("ENTRA_CLIENT_ID", ""),
		EntraAPIScope:    getEnv("ENTRA_API_SCOPE", ""),
		EntraRedirectURI: getEnv("ENTRA_REDIRECT_URI", ""),
	}
}

func (c *Config) Validate() error {
	if c.AuthMode != "disabled" && c.AuthMode != "entra" {
		return fmt.Errorf("unknown AUTH_MODE %q, must be \"disabled\" or \"entra\"", c.AuthMode)
	}
	if c.AuthMode == "entra" {
		if c.EntraTenantID == "" {
			return fmt.Errorf("ENTRA_TENANT_ID is required when AUTH_MODE is \"entra\"")
		}
		if c.EntraClientID == "" {
			return fmt.Errorf("ENTRA_CLIENT_ID is required when AUTH_MODE is \"entra\"")
		}
		if c.EntraAPIScope == "" {
			return fmt.Errorf("ENTRA_API_SCOPE is required when AUTH_MODE is \"entra\"")
		}
		if c.EntraRedirectURI == "" {
			return fmt.Errorf("ENTRA_REDIRECT_URI is required when AUTH_MODE is \"entra\"")
		}
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
