package database

import (
	"fmt"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/config"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// BaseModel is a replacement for gorm.Model that uses a string UUID as the primary key.
type BaseModel struct {
	ID        string         `gorm:"type:char(36);primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID for new records.
func (b *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

func Connect(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.DBDriver {
	case "sqlite":
		dialector = sqlite.Open(cfg.DatabaseURL)
	case "postgres":
		dialector = postgres.Open(cfg.DatabaseURL)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.DBDriver)
	}

	// Silence GORM's default logger. It uses stderr with blocking writes and
	// logs every ErrRecordNotFound as a warning — which is normal lookup
	// behavior here, not an error. Under e2e load the 64 KB stderr pipe
	// buffer fills, `os.File.Write` blocks inside the kernel write syscall
	// while holding the fd-mutex, and every subsequent HTTP handler that
	// touches the DB wedges waiting for the same lock. This deadlocked the
	// full Playwright suite reliably at ~40-80 requests.
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}
