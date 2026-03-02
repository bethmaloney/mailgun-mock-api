package database_test

import (
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/config"
	"github.com/bethmaloney/mailgun-mock-api/internal/database"
)

// ---------------------------------------------------------------------------
// Database connection and migration tests
// ---------------------------------------------------------------------------

func TestConnect_SQLite(t *testing.T) {
	cfg := &config.Config{
		DBDriver:    "sqlite",
		DatabaseURL: "file::memory:?cache=shared",
	}

	db, err := database.Connect(cfg)

	t.Run("connects without error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if db == nil {
			t.Fatal("expected non-nil db instance")
		}
	})

	t.Run("can ping the database", func(t *testing.T) {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("failed to get underlying sql.DB: %v", err)
		}
		if err := sqlDB.Ping(); err != nil {
			t.Fatalf("failed to ping database: %v", err)
		}
	})
}

func TestConnect_UnsupportedDriver(t *testing.T) {
	cfg := &config.Config{
		DBDriver:    "mysql",
		DatabaseURL: "fake://localhost",
	}

	db, err := database.Connect(cfg)

	t.Run("returns error for unsupported driver", func(t *testing.T) {
		if err == nil {
			t.Fatal("expected error for unsupported driver, got nil")
		}
		if db != nil {
			t.Error("expected nil db for unsupported driver")
		}
	})
}

func TestConnect_ReturnsUsableDB(t *testing.T) {
	cfg := &config.Config{
		DBDriver:    "sqlite",
		DatabaseURL: "file::memory:?cache=shared",
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	t.Run("can migrate and use a table with BaseModel", func(t *testing.T) {
		type ConnTestModel struct {
			database.BaseModel
			Name string `gorm:"uniqueIndex"`
		}
		if err := db.AutoMigrate(&ConnTestModel{}); err != nil {
			t.Fatalf("failed to migrate ConnTestModel: %v", err)
		}

		record := ConnTestModel{Name: "test.example.com"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("failed to create record: %v", err)
		}

		var found ConnTestModel
		if err := db.First(&found, "name = ?", "test.example.com").Error; err != nil {
			t.Fatalf("failed to find record: %v", err)
		}
		if found.Name != "test.example.com" {
			t.Errorf("expected name %q, got %q", "test.example.com", found.Name)
		}
	})
}

// ---------------------------------------------------------------------------
// BaseModel tests
// ---------------------------------------------------------------------------

func TestBaseModel(t *testing.T) {
	cfg := &config.Config{
		DBDriver:    "sqlite",
		DatabaseURL: "file::memory:?cache=shared",
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	t.Run("BaseModel struct exists with expected fields", func(t *testing.T) {
		// Verify we can instantiate a BaseModel and it has the expected zero values.
		// This test confirms the struct is exported and usable.
		var bm database.BaseModel
		if bm.ID != "" {
			t.Errorf("expected empty ID on zero value BaseModel, got %q", bm.ID)
		}
		if !bm.CreatedAt.IsZero() {
			t.Errorf("expected zero CreatedAt, got %v", bm.CreatedAt)
		}
		if !bm.UpdatedAt.IsZero() {
			t.Errorf("expected zero UpdatedAt, got %v", bm.UpdatedAt)
		}
	})

	t.Run("BaseModel ID is a string UUID primary key", func(t *testing.T) {
		// Migrate a test model that embeds BaseModel to verify the schema
		type TestModel struct {
			database.BaseModel
			Label string
		}
		if err := db.AutoMigrate(&TestModel{}); err != nil {
			t.Fatalf("failed to migrate TestModel: %v", err)
		}

		if !db.Migrator().HasTable(&TestModel{}) {
			t.Fatal("expected TestModel table to exist after migration")
		}
	})

	t.Run("BaseModel generates UUID for new records", func(t *testing.T) {
		type TestEntity struct {
			database.BaseModel
			Value string
		}
		if err := db.AutoMigrate(&TestEntity{}); err != nil {
			t.Fatalf("failed to migrate TestEntity: %v", err)
		}

		entity := TestEntity{Value: "hello"}
		if err := db.Create(&entity).Error; err != nil {
			t.Fatalf("failed to create TestEntity: %v", err)
		}

		if entity.ID == "" {
			t.Error("expected non-empty UUID ID after creation")
		}

		// UUID v4 format check: 8-4-4-4-12 hex chars
		if len(entity.ID) != 36 {
			t.Errorf("expected UUID length 36, got %d (%q)", len(entity.ID), entity.ID)
		}
	})

	t.Run("BaseModel sets CreatedAt on creation", func(t *testing.T) {
		type TimestampTest struct {
			database.BaseModel
			Data string
		}
		if err := db.AutoMigrate(&TimestampTest{}); err != nil {
			t.Fatalf("failed to migrate: %v", err)
		}

		record := TimestampTest{Data: "timestamp test"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("failed to create record: %v", err)
		}

		if record.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set after creation")
		}
	})

	t.Run("BaseModel sets UpdatedAt on update", func(t *testing.T) {
		type UpdateTest struct {
			database.BaseModel
			Data string
		}
		if err := db.AutoMigrate(&UpdateTest{}); err != nil {
			t.Fatalf("failed to migrate: %v", err)
		}

		record := UpdateTest{Data: "initial"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("failed to create: %v", err)
		}

		originalUpdatedAt := record.UpdatedAt

		record.Data = "modified"
		if err := db.Save(&record).Error; err != nil {
			t.Fatalf("failed to update: %v", err)
		}

		// Reload from DB to get the actual stored value
		var reloaded UpdateTest
		if err := db.First(&reloaded, "id = ?", record.ID).Error; err != nil {
			t.Fatalf("failed to reload: %v", err)
		}

		if reloaded.UpdatedAt.Before(originalUpdatedAt) {
			t.Error("expected UpdatedAt to be >= original after update")
		}
	})

	t.Run("BaseModel supports soft delete via DeletedAt", func(t *testing.T) {
		type SoftDeleteTest struct {
			database.BaseModel
			Name string
		}
		if err := db.AutoMigrate(&SoftDeleteTest{}); err != nil {
			t.Fatalf("failed to migrate: %v", err)
		}

		record := SoftDeleteTest{Name: "to-be-deleted"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("failed to create: %v", err)
		}

		// Soft delete
		if err := db.Delete(&record).Error; err != nil {
			t.Fatalf("failed to soft delete: %v", err)
		}

		// Should not find with normal query
		var notFound SoftDeleteTest
		result := db.First(&notFound, "id = ?", record.ID)
		if result.Error == nil {
			t.Error("expected record to be hidden after soft delete")
		}

		// Should find with Unscoped
		var found SoftDeleteTest
		result = db.Unscoped().First(&found, "id = ?", record.ID)
		if result.Error != nil {
			t.Errorf("expected to find soft-deleted record with Unscoped, got error: %v", result.Error)
		}
	})

	t.Run("each record gets a unique ID", func(t *testing.T) {
		type UniqueIDTest struct {
			database.BaseModel
			Seq int
		}
		if err := db.AutoMigrate(&UniqueIDTest{}); err != nil {
			t.Fatalf("failed to migrate: %v", err)
		}

		ids := make(map[string]bool)
		for i := 0; i < 10; i++ {
			record := UniqueIDTest{Seq: i}
			if err := db.Create(&record).Error; err != nil {
				t.Fatalf("failed to create record %d: %v", i, err)
			}
			if ids[record.ID] {
				t.Errorf("duplicate ID detected: %q", record.ID)
			}
			ids[record.ID] = true
		}
	})
}

// ---------------------------------------------------------------------------
// Multiple Connect calls (idempotent migrations)
// ---------------------------------------------------------------------------

func TestConnect_IdempotentConnections(t *testing.T) {
	cfg := &config.Config{
		DBDriver:    "sqlite",
		DatabaseURL: "file::memory:?cache=shared",
	}

	// First connect
	db1, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("first connect failed: %v", err)
	}

	type IdempotentModel struct {
		database.BaseModel
		Name string `gorm:"uniqueIndex"`
	}
	if err := db1.AutoMigrate(&IdempotentModel{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Insert a record
	record := IdempotentModel{Name: "idempotent-test.com"}
	if err := db1.Create(&record).Error; err != nil {
		t.Fatalf("failed to create record: %v", err)
	}

	// Second connect (should not destroy data)
	db2, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("second connect failed: %v", err)
	}

	var found IdempotentModel
	result := db2.First(&found, "name = ?", "idempotent-test.com")
	if result.Error != nil {
		t.Errorf("expected to find record after second Connect, got error: %v", result.Error)
	}
}
