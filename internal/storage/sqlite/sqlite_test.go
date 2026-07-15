package sqlite

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestOpenAppliesEmbeddedMigrations(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "traceframe.db")
	db, err := Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var migrationCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	if migrationCount != len(entries) {
		t.Fatalf("migration count = %d, want %d", migrationCount, len(entries))
	}

	var initialized string
	if err := db.QueryRow("SELECT value FROM application_metadata WHERE key = 'schema_initialized'").Scan(&initialized); err != nil {
		t.Fatalf("query application_metadata: %v", err)
	}
	if initialized != "true" {
		t.Fatalf("schema_initialized = %q, want true", initialized)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "traceframe.db")
	for range 2 {
		db, err := Open(context.Background(), databasePath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}
