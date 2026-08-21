package database

import (
	"path/filepath"
	"testing"
)

func TestOpenEnablesWALAndMigrationTable(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "lan-drop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode;").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("expected WAL mode, got %q", mode)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version IN (1, 2, 3, 4, 5)").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("expected 5 applied migrations, got %d", count)
	}

	// Verify rooms table exists and can be queried
	var roomCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&roomCount); err != nil {
		t.Fatalf("query rooms table failed: %v", err)
	}
	if roomCount != 0 {
		t.Fatalf("expected 0 rooms initially, got %d", roomCount)
	}

	// Verify files table exists and can be queried
	var fileCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount); err != nil {
		t.Fatalf("query files table failed: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected 0 files initially, got %d", fileCount)
	}

	// Verify room_sessions table exists and can be queried
	var sessionCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM room_sessions").Scan(&sessionCount); err != nil {
		t.Fatalf("query room_sessions table failed: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected 0 room_sessions initially, got %d", sessionCount)
	}

	// Verify shares table exists and can be queried
	var shareCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM shares").Scan(&shareCount); err != nil {
		t.Fatalf("query shares table failed: %v", err)
	}
	if shareCount != 0 {
		t.Fatalf("expected 0 shares initially, got %d", shareCount)
	}
}
