package config

import (
	"strings"
	"testing"
)

func TestLoad_DefaultAuthMode(t *testing.T) {
	t.Setenv("AUTH_MODE", "")
	cfg := Load()

	if cfg.AuthMode != "disabled" {
		t.Errorf("expected AuthMode to default to %q, got %q", "disabled", cfg.AuthMode)
	}
}

func TestLoad_EntraAuthMode(t *testing.T) {
	t.Setenv("AUTH_MODE", "entra")
	t.Setenv("ENTRA_TENANT_ID", "tenant-123")
	t.Setenv("ENTRA_CLIENT_ID", "client-456")
	t.Setenv("ENTRA_API_SCOPE", "api://mock-mailgun/.default")
	t.Setenv("ENTRA_REDIRECT_URI", "http://localhost:8025/auth/callback")

	cfg := Load()

	if cfg.AuthMode != "entra" {
		t.Errorf("expected AuthMode %q, got %q", "entra", cfg.AuthMode)
	}
	if cfg.EntraTenantID != "tenant-123" {
		t.Errorf("expected EntraTenantID %q, got %q", "tenant-123", cfg.EntraTenantID)
	}
	if cfg.EntraClientID != "client-456" {
		t.Errorf("expected EntraClientID %q, got %q", "client-456", cfg.EntraClientID)
	}
	if cfg.EntraAPIScope != "api://mock-mailgun/.default" {
		t.Errorf("expected EntraAPIScope %q, got %q", "api://mock-mailgun/.default", cfg.EntraAPIScope)
	}
	if cfg.EntraRedirectURI != "http://localhost:8025/auth/callback" {
		t.Errorf("expected EntraRedirectURI %q, got %q", "http://localhost:8025/auth/callback", cfg.EntraRedirectURI)
	}
}

func TestValidate_DisabledMode(t *testing.T) {
	cfg := &Config{
		AuthMode: "disabled",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for disabled mode, got: %v", err)
	}
}

func TestValidate_EntraMode_AllSet(t *testing.T) {
	cfg := &Config{
		AuthMode:        "entra",
		EntraTenantID:   "tenant-123",
		EntraClientID:   "client-456",
		EntraAPIScope:   "api://mock-mailgun/.default",
		EntraRedirectURI: "http://localhost:8025/auth/callback",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error when all Entra fields set, got: %v", err)
	}
}

func TestValidate_EntraMode_MissingTenantID(t *testing.T) {
	cfg := &Config{
		AuthMode:        "entra",
		EntraTenantID:   "",
		EntraClientID:   "client-456",
		EntraAPIScope:   "api://mock-mailgun/.default",
		EntraRedirectURI: "http://localhost:8025/auth/callback",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when EntraTenantID is missing, got nil")
	}
	if !strings.Contains(err.Error(), "ENTRA_TENANT_ID") {
		t.Errorf("expected error to mention ENTRA_TENANT_ID, got: %v", err)
	}
}

func TestValidate_EntraMode_MissingClientID(t *testing.T) {
	cfg := &Config{
		AuthMode:        "entra",
		EntraTenantID:   "tenant-123",
		EntraClientID:   "",
		EntraAPIScope:   "api://mock-mailgun/.default",
		EntraRedirectURI: "http://localhost:8025/auth/callback",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when EntraClientID is missing, got nil")
	}
	if !strings.Contains(err.Error(), "ENTRA_CLIENT_ID") {
		t.Errorf("expected error to mention ENTRA_CLIENT_ID, got: %v", err)
	}
}

func TestValidate_EntraMode_MissingAPIScope(t *testing.T) {
	cfg := &Config{
		AuthMode:        "entra",
		EntraTenantID:   "tenant-123",
		EntraClientID:   "client-456",
		EntraAPIScope:   "",
		EntraRedirectURI: "http://localhost:8025/auth/callback",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when EntraAPIScope is missing, got nil")
	}
	if !strings.Contains(err.Error(), "ENTRA_API_SCOPE") {
		t.Errorf("expected error to mention ENTRA_API_SCOPE, got: %v", err)
	}
}

func TestValidate_EntraMode_MissingRedirectURI(t *testing.T) {
	cfg := &Config{
		AuthMode:        "entra",
		EntraTenantID:   "tenant-123",
		EntraClientID:   "client-456",
		EntraAPIScope:   "api://mock-mailgun/.default",
		EntraRedirectURI: "",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when EntraRedirectURI is missing, got nil")
	}
	if !strings.Contains(err.Error(), "ENTRA_REDIRECT_URI") {
		t.Errorf("expected error to mention ENTRA_REDIRECT_URI, got: %v", err)
	}
}

func TestValidate_UnknownAuthMode(t *testing.T) {
	cfg := &Config{
		AuthMode: "typo",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown AuthMode, got nil")
	}
	if !strings.Contains(err.Error(), "unknown AUTH_MODE") {
		t.Errorf("expected error to mention \"unknown AUTH_MODE\", got: %v", err)
	}
}
