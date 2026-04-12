package apikey

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
)

// ManagedAPIKey represents a managed API key created through the mock UI.
type ManagedAPIKey struct {
	database.BaseModel
	Name     string `gorm:"not null" json:"name"`
	KeyValue string `gorm:"uniqueIndex;not null" json:"key_value"`
	Prefix   string `gorm:"not null" json:"prefix"`
}

// generateManagedKeyValue generates a new managed API key value and its prefix.
// The value is "mock_" followed by 32 random bytes encoded as base64url (no padding).
// The prefix is the first 13 characters of the value.
func generateManagedKeyValue() (value, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("crypto/rand.Read failed: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(b)
	value = "mock_" + encoded
	prefix = value[:13]
	return value, prefix, nil
}
