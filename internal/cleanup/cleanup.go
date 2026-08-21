package cleanup

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/i1k3r/lan-drop/internal/storage"
)

type Options struct {
	Interval            time.Duration
	BatchSize           int
	StagingMaxAge       time.Duration
	OrphanGracePeriod   time.Duration
	ClosedRoomRetention time.Duration
}

func DefaultOptions() Options {
	return Options{
		Interval:            time.Minute,
		BatchSize:           50,
		StagingMaxAge:       15 * time.Minute,
		OrphanGracePeriod:   10 * time.Minute,
		ClosedRoomRetention: 0,
	}
}

type Stats struct {
	RoomsScanned    int           `json:"rooms_scanned"`
	RoomsCleaned    int           `json:"rooms_cleaned"`
	FilesDeleted    int           `json:"files_deleted"`
	StagingCleaned  int           `json:"staging_cleaned"`
	OrphansCleaned  int           `json:"orphans_cleaned"`
	SessionsCleaned int           `json:"sessions_cleaned"`
	SharesCleaned   int           `json:"shares_cleaned"`
	Errors          int           `json:"errors"`
	Duration        time.Duration `json:"duration"`
}

type Worker struct {
	db     *sql.DB
	paths  storage.Paths
	opts   Options
	logger *slog.Logger
}

func NewWorker(db *sql.DB, paths storage.Paths, opts Options, logger *slog.Logger) *Worker {
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 50
	}
	if opts.StagingMaxAge <= 0 {
		opts.StagingMaxAge = 15 * time.Minute
	}
	if opts.OrphanGracePeriod <= 0 {
		opts.OrphanGracePeriod = 10 * time.Minute
	}
	if opts.ClosedRoomRetention < 0 {
		opts.ClosedRoomRetention = 0
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		db:     db,
		paths:  paths,
		opts:   opts,
		logger: logger,
	}
}

// Run executes the background cleanup loop until ctx is canceled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()

	w.logger.Info("cleanup worker started", "interval", w.opts.Interval.String(), "batch_size", w.opts.BatchSize)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("cleanup worker stopping")
			return
		case <-ticker.C:
			_, _ = w.RunOnce(ctx)
		}
	}
}

// RunOnce performs a single complete cleanup pass.
func (w *Worker) RunOnce(ctx context.Context) (Stats, error) {
	start := time.Now()
	var stats Stats

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	closedCutoffStr := now.Add(-w.opts.ClosedRoomRetention).Format(time.RFC3339)

	// Step 1: Expired & eligible closed room cleanup
	roomQuery := `
		SELECT id
		FROM rooms
		WHERE expires_at <= ? OR (status = 'closed' AND created_at <= ?)
		LIMIT ?;
	`
	rows, err := w.db.QueryContext(ctx, roomQuery, nowStr, closedCutoffStr, w.opts.BatchSize)
	if err != nil {
		stats.Errors++
		w.logger.Error("failed to query rooms for cleanup", "error", err)
	} else {
		var roomIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				roomIDs = append(roomIDs, id)
			}
		}
		rows.Close()
		stats.RoomsScanned = len(roomIDs)

		for _, roomID := range roomIDs {
			if err := ctx.Err(); err != nil {
				break
			}
			filesDeleted, err := w.cleanRoom(ctx, roomID)
			if err != nil {
				stats.Errors++
				w.logger.Error("failed to clean room", "room_id", roomID, "error", err)
				continue
			}
			stats.RoomsCleaned++
			stats.FilesDeleted += filesDeleted
		}
	}

	// Step 2: Stale /data/staging .tmp cleanup (interrupted / crashed uploads)
	if err := ctx.Err(); err == nil {
		stagingCleaned, err := w.cleanStaging()
		if err != nil {
			stats.Errors++
			w.logger.Error("failed to clean staging directory", "error", err)
		} else {
			stats.StagingCleaned = stagingCleaned
		}
	}

	// Step 3: Orphan /data/files reconciliation (unindexed finalized files)
	if err := ctx.Err(); err == nil {
		orphansCleaned, err := w.reconcileOrphanFiles(ctx)
		if err != nil {
			stats.Errors++
			w.logger.Error("failed to reconcile orphan files", "error", err)
		} else {
			stats.OrphansCleaned = orphansCleaned
		}
	}

	// Step 4: Standalone expired room sessions sweep
	if err := ctx.Err(); err == nil {
		res, err := w.db.ExecContext(ctx, "DELETE FROM room_sessions WHERE expires_at <= ?", nowStr)
		if err != nil {
			stats.Errors++
			w.logger.Error("failed to purge expired sessions", "error", err)
		} else if rowsAffected, err := res.RowsAffected(); err == nil {
			stats.SessionsCleaned = int(rowsAffected)
		}
	}

	// Step 5: Standalone expired or revoked shares sweep
	if err := ctx.Err(); err == nil {
		res, err := w.db.ExecContext(ctx, "DELETE FROM shares WHERE expires_at <= ? OR status = 'revoked'", nowStr)
		if err != nil {
			stats.Errors++
			w.logger.Error("failed to purge expired/revoked shares", "error", err)
		} else if rowsAffected, err := res.RowsAffected(); err == nil {
			stats.SharesCleaned = int(rowsAffected)
		}
	}

	stats.Duration = time.Since(start)

	w.logger.Info("cleanup completed",
		"rooms_scanned", stats.RoomsScanned,
		"rooms_cleaned", stats.RoomsCleaned,
		"files_deleted", stats.FilesDeleted,
		"staging_cleaned", stats.StagingCleaned,
		"orphans_cleaned", stats.OrphansCleaned,
		"sessions_cleaned", stats.SessionsCleaned,
		"shares_cleaned", stats.SharesCleaned,
		"errors", stats.Errors,
		"duration_ms", stats.Duration.Milliseconds(),
	)

	return stats, nil
}

// cleanRoom performs an atomic per-room database deletion and physical file deletion.
func (w *Worker) cleanRoom(ctx context.Context, roomID string) (int, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Collect associated storage IDs
	fileRows, err := tx.QueryContext(ctx, "SELECT storage_id FROM files WHERE room_id = ?", roomID)
	if err != nil {
		return 0, fmt.Errorf("query storage ids: %w", err)
	}
	var storageIDs []string
	for fileRows.Next() {
		var sID string
		if err := fileRows.Scan(&sID); err == nil {
			storageIDs = append(storageIDs, sID)
		}
	}
	fileRows.Close()

	// 2. Delete room from SQLite (ON DELETE CASCADE removes files and room_sessions)
	res, err := tx.ExecContext(ctx, "DELETE FROM rooms WHERE id = ?", roomID)
	if err != nil {
		return 0, fmt.Errorf("delete room: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rowsAffected == 0 {
		return 0, nil
	}

	// 3. Commit transaction
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit room deletion: %w", err)
	}

	// 4. Delete physical files from disk
	deletedCount := 0
	for _, sID := range storageIDs {
		if !isValidStorageID(sID) {
			continue
		}
		targetPath := filepath.Clean(filepath.Join(w.paths.FilesDir, sID))
		if !isContainedIn(w.paths.FilesDir, targetPath) {
			continue
		}
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			w.logger.Warn("failed to remove physical file", "storage_id", sID, "error", err)
		} else if err == nil {
			deletedCount++
		}
	}

	return deletedCount, nil
}

// cleanStaging purges temporary upload files in /data/staging older than StagingMaxAge.
func (w *Worker) cleanStaging() (int, error) {
	entries, err := os.ReadDir(w.paths.StagingDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read staging dir: %w", err)
	}

	cleaned := 0
	for _, entry := range entries {
		path := filepath.Clean(filepath.Join(w.paths.StagingDir, entry.Name()))
		if !isContainedIn(w.paths.StagingDir, path) {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		// If entry is a symlink or file older than StagingMaxAge, remove it
		if info.Mode()&os.ModeSymlink != 0 || time.Since(info.ModTime()) > w.opts.StagingMaxAge {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				w.logger.Warn("failed to remove stale staging file", "file", entry.Name(), "error", err)
			} else if err == nil {
				cleaned++
			}
		}
	}
	return cleaned, nil
}

// reconcileOrphanFiles removes unindexed physical files in /data/files older than OrphanGracePeriod.
func (w *Worker) reconcileOrphanFiles(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(w.paths.FilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read files dir: %w", err)
	}

	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		storageID := entry.Name()
		if !isValidStorageID(storageID) {
			continue
		}
		path := filepath.Clean(filepath.Join(w.paths.FilesDir, storageID))
		if !isContainedIn(w.paths.FilesDir, path) {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}

		// Only consider files older than the grace period to avoid racing with in-flight finalization
		if time.Since(info.ModTime()) <= w.opts.OrphanGracePeriod {
			continue
		}

		// Check if record exists in database with status 'ready'
		var exists int
		err = w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files WHERE storage_id = ? AND status = 'ready'", storageID).Scan(&exists)
		if err != nil {
			w.logger.Warn("failed to query file existence for orphan check", "storage_id", storageID, "error", err)
			continue
		}

		if exists == 0 {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				w.logger.Warn("failed to remove orphan file", "storage_id", storageID, "error", err)
			} else if err == nil {
				cleaned++
			}
		}
	}
	return cleaned, nil
}

func isValidStorageID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func isContainedIn(baseDir, targetPath string) bool {
	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
