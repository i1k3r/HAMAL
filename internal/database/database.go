package database

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", filepath.ToSlash(path)+"?_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);`)
	if err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (1) ON CONFLICT(version) DO NOTHING;`); err != nil {
		return fmt.Errorf("record foundation migration: %w", err)
	}

	var v2Applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 2;`).Scan(&v2Applied); err != nil {
		return fmt.Errorf("check migration 2: %w", err)
	}
	if v2Applied == 0 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration 2: %w", err)
		}
		defer tx.Rollback()
		schema := `
			CREATE TABLE IF NOT EXISTS rooms (
				id TEXT PRIMARY KEY,
				creator_token_hash TEXT NOT NULL UNIQUE,
				participant_token_hash TEXT NOT NULL UNIQUE,
				created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
				expires_at TEXT NOT NULL,
				ttl_seconds INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'active',
				pin_required INTEGER NOT NULL DEFAULT 0,
				pin_hash TEXT NULL,
				pin_salt TEXT NULL,
				pin_attempts INTEGER NOT NULL DEFAULT 0,
				max_room_size INTEGER NOT NULL,
				max_file_size INTEGER NOT NULL,
				max_files INTEGER NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_rooms_expires_at ON rooms (expires_at);
			CREATE INDEX IF NOT EXISTS idx_rooms_creator_hash ON rooms (creator_token_hash);
			CREATE INDEX IF NOT EXISTS idx_rooms_participant_hash ON rooms (participant_token_hash);
			CREATE INDEX IF NOT EXISTS idx_rooms_status ON rooms (status);
			INSERT INTO schema_migrations (version) VALUES (2);
		`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("execute migration 2: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration 2: %w", err)
		}
	}

	var v3Applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 3;`).Scan(&v3Applied); err != nil {
		return fmt.Errorf("check migration 3: %w", err)
	}
	if v3Applied == 0 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration 3: %w", err)
		}
		defer tx.Rollback()
		schema := `
			CREATE TABLE IF NOT EXISTS files (
				id TEXT PRIMARY KEY,
				room_id TEXT NOT NULL,
				storage_id TEXT NOT NULL UNIQUE,
				original_filename TEXT NOT NULL,
				size_bytes INTEGER NOT NULL,
				content_type TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'ready',
				created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
				completed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
				FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_files_room_id ON files (room_id);
			CREATE INDEX IF NOT EXISTS idx_files_storage_id ON files (storage_id);
			CREATE INDEX IF NOT EXISTS idx_files_status ON files (status);
			CREATE INDEX IF NOT EXISTS idx_files_created_at ON files (created_at);
			INSERT INTO schema_migrations (version) VALUES (3);
		`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("execute migration 3: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration 3: %w", err)
		}
	}

	var v4Applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 4;`).Scan(&v4Applied); err != nil {
		return fmt.Errorf("check migration 4: %w", err)
	}
	if v4Applied == 0 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration 4: %w", err)
		}
		defer tx.Rollback()
		schema := `
			ALTER TABLE rooms ADD COLUMN locked_until TEXT NULL;

			CREATE TABLE IF NOT EXISTS room_sessions (
				id TEXT PRIMARY KEY,
				room_id TEXT NOT NULL,
				session_token_hash TEXT NOT NULL UNIQUE,
				created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
				expires_at TEXT NOT NULL,
				FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_room_sessions_token_hash ON room_sessions (session_token_hash);
			CREATE INDEX IF NOT EXISTS idx_room_sessions_room_id ON room_sessions (room_id);
			INSERT INTO schema_migrations (version) VALUES (4);
		`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("execute migration 4: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration 4: %w", err)
		}
	}

	var v5Applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 5;`).Scan(&v5Applied); err != nil {
		return fmt.Errorf("check migration 5: %w", err)
	}
	if v5Applied == 0 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration 5: %w", err)
		}
		defer tx.Rollback()
		schema := `
			CREATE TABLE IF NOT EXISTS shares (
				id TEXT PRIMARY KEY,
				room_id TEXT NOT NULL,
				file_id TEXT NOT NULL,
				token_hash TEXT NOT NULL UNIQUE,
				status TEXT NOT NULL DEFAULT 'active',
				created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
				expires_at TEXT NOT NULL,
				revoked_at TEXT NULL,
				FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
				FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_shares_token_hash ON shares (token_hash);
			CREATE INDEX IF NOT EXISTS idx_shares_room_id ON shares (room_id);
			CREATE INDEX IF NOT EXISTS idx_shares_file_id ON shares (file_id);
			CREATE INDEX IF NOT EXISTS idx_shares_expires_at ON shares (expires_at);
			CREATE INDEX IF NOT EXISTS idx_shares_status ON shares (status);
			INSERT INTO schema_migrations (version) VALUES (5);
		`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("execute migration 5: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration 5: %w", err)
		}
	}
	return nil
}

func Check(db *sql.DB) error {
	if err := db.Ping(); err != nil {
		return fmt.Errorf("sqlite unavailable: %w", err)
	}
	return nil
}
