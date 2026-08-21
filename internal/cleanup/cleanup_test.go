package cleanup

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/i1k3r/lan-drop/internal/database"
	"github.com/i1k3r/lan-drop/internal/file"
	"github.com/i1k3r/lan-drop/internal/room"
	"github.com/i1k3r/lan-drop/internal/share"
	"github.com/i1k3r/lan-drop/internal/storage"
)

type testHarness struct {
	db         *sql.DB
	paths      storage.Paths
	roomStore  *room.Store
	fileStore  *file.Store
	shareStore *share.Store
	worker     *Worker
	logger     *slog.Logger
}

func setupTestHarness(t *testing.T, opts Options) *testHarness {
	t.Helper()
	dataDir := t.TempDir()
	paths, err := storage.Initialize(dataDir)
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(filepath.Join(dataDir, "lan-drop.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	secret := "test-secret-at-least-32-bytes-long-for-tests"
	roomStore := room.NewStore(db, secret)
	quotaMgr := file.NewQuotaManager()
	fileStore := file.NewStore(db, paths, quotaMgr)
	shareStore := share.NewStore(db, secret)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := NewWorker(db, paths, opts, logger)

	return &testHarness{
		db:         db,
		paths:      paths,
		roomStore:  roomStore,
		fileStore:  fileStore,
		shareStore: shareStore,
		worker:     worker,
		logger:     logger,
	}
}

func TestCleanupExpiredRooms(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	// 1. Create 2 rooms: one expired, one active
	expiredRoom, err := h.roomStore.Create(ctx, -time.Minute, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	activeRoom, err := h.roomStore.Create(ctx, 30*time.Minute, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Upload file to expired room
	content := []byte("expired content")
	upExpired, err := h.fileStore.StreamUpload(ctx, expiredRoom.ID, "expired.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Upload file to active room
	activeContent := []byte("active content")
	upActive, err := h.fileStore.StreamUpload(ctx, activeRoom.ID, "active.txt", "text/plain", bytes.NewReader(activeContent), int64(len(activeContent)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Run cleanup once
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if stats.RoomsCleaned != 1 {
		t.Fatalf("expected 1 room cleaned, got %d", stats.RoomsCleaned)
	}
	if stats.FilesDeleted != 1 {
		t.Fatalf("expected 1 file deleted, got %d", stats.FilesDeleted)
	}

	// Verify expired room file is deleted from disk
	if _, err := os.Stat(filepath.Join(h.paths.FilesDir, upExpired.StorageID)); !os.IsNotExist(err) {
		t.Fatalf("expected expired file on disk to be removed")
	}

	// Verify active room file remains on disk
	if _, err := os.Stat(filepath.Join(h.paths.FilesDir, upActive.StorageID)); err != nil {
		t.Fatalf("active room file must not be removed: %v", err)
	}

	// Verify database rows
	var activeCount int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM rooms WHERE id = ?", activeRoom.ID).Scan(&activeCount); err != nil || activeCount != 1 {
		t.Fatalf("active room record must still exist")
	}
	var expiredCount int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM rooms WHERE id = ?", expiredRoom.ID).Scan(&expiredCount); err != nil || expiredCount != 0 {
		t.Fatalf("expired room record must be deleted")
	}
}

func TestClosedRoomRetentionPolicy(t *testing.T) {
	opts := DefaultOptions()
	opts.ClosedRoomRetention = 10 * time.Minute
	h := setupTestHarness(t, opts)
	ctx := context.Background()

	// Create room and close it immediately
	rm, err := h.roomStore.Create(ctx, time.Hour, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.roomStore.Close(ctx, rm.CreatorToken); err != nil {
		t.Fatal(err)
	}

	content := []byte("closed room content")
	up, err := h.fileStore.StreamUpload(ctx, rm.ID, "closed.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	// First pass: closed room was created just now, retention is 10m -> should NOT be cleaned yet
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 0 {
		t.Fatalf("expected 0 rooms cleaned during retention window, got %d", stats.RoomsCleaned)
	}
	if _, err := os.Stat(filepath.Join(h.paths.FilesDir, up.StorageID)); err != nil {
		t.Fatalf("file must remain intact during retention window")
	}

	// Manually set room's created_at to 15 minutes ago
	oldCreatedAt := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)
	if _, err := h.db.Exec("UPDATE rooms SET created_at = ? WHERE id = ?", oldCreatedAt, rm.ID); err != nil {
		t.Fatal(err)
	}

	// Second pass: closed room is older than 10m retention -> MUST be cleaned
	stats, err = h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 1 {
		t.Fatalf("expected 1 closed room cleaned after retention window, got %d", stats.RoomsCleaned)
	}
	if _, err := os.Stat(filepath.Join(h.paths.FilesDir, up.StorageID)); !os.IsNotExist(err) {
		t.Fatalf("closed room file must be removed after retention window")
	}
}

func TestRoomSessionsCascadingDeletion(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	rm, err := h.roomStore.Create(ctx, -time.Minute, 10<<20, 2<<20, 10, "1234")
	if err != nil {
		t.Fatal(err)
	}

	// Create session token for room
	_, err = h.roomStore.CreateSession(ctx, rm.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	// Verify session exists
	var sessionCount int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM room_sessions WHERE room_id = ?", rm.ID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 {
		t.Fatalf("expected 1 session before cleanup, got %d", sessionCount)
	}

	// Run cleanup
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 1 {
		t.Fatalf("expected 1 room cleaned, got %d", stats.RoomsCleaned)
	}

	// Verify room_sessions was deleted via ON DELETE CASCADE
	if err := h.db.QueryRow("SELECT COUNT(*) FROM room_sessions WHERE room_id = ?", rm.ID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected 0 sessions after room cleanup, got %d", sessionCount)
	}
}

func TestStandaloneExpiredSessionSweep(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	// Active room
	rm, err := h.roomStore.Create(ctx, time.Hour, 10<<20, 2<<20, 10, "1234")
	if err != nil {
		t.Fatal(err)
	}

	// Create expired session
	_, err = h.roomStore.CreateSession(ctx, rm.ID, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	// Create valid active session
	_, err = h.roomStore.CreateSession(ctx, rm.ID, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SessionsCleaned != 1 {
		t.Fatalf("expected 1 standalone expired session cleaned, got %d", stats.SessionsCleaned)
	}

	// Active session must remain
	var count int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM room_sessions WHERE room_id = ?", rm.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 active session remaining, got %d", count)
	}
}

func TestCleanStagingPurgesStaleTempFiles(t *testing.T) {
	opts := DefaultOptions()
	opts.StagingMaxAge = 10 * time.Minute
	h := setupTestHarness(t, opts)
	ctx := context.Background()

	// 1. Create fresh staging file (modified just now)
	freshFile := filepath.Join(h.paths.StagingDir, "fresh.tmp")
	if err := os.WriteFile(freshFile, []byte("fresh in-flight upload"), 0600); err != nil {
		t.Fatal(err)
	}

	// 2. Create stale staging file (modified 20 minutes ago)
	staleFile := filepath.Join(h.paths.StagingDir, "stale.tmp")
	if err := os.WriteFile(staleFile, []byte("stale abandoned upload"), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-20 * time.Minute)
	if err := os.Chtimes(staleFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.StagingCleaned != 1 {
		t.Fatalf("expected 1 stale staging file cleaned, got %d", stats.StagingCleaned)
	}

	// Verify fresh file remains intact
	if _, err := os.Stat(freshFile); err != nil {
		t.Fatalf("fresh staging file must not be removed: %v", err)
	}

	// Verify stale file was removed
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Fatalf("stale staging file must be removed")
	}
}

func TestOrphanReconciliationPurgesUnindexedFiles(t *testing.T) {
	opts := DefaultOptions()
	opts.OrphanGracePeriod = 5 * time.Minute
	h := setupTestHarness(t, opts)
	ctx := context.Background()

	// 1. Create fresh unindexed file (modified 1 minute ago, within grace period)
	freshOrphan := filepath.Join(h.paths.FilesDir, "fresh_storage_id")
	if err := os.WriteFile(freshOrphan, []byte("in flight file"), 0600); err != nil {
		t.Fatal(err)
	}
	freshTime := time.Now().Add(-time.Minute)
	_ = os.Chtimes(freshOrphan, freshTime, freshTime)

	// 2. Create old unindexed file (modified 10 minutes ago, past grace period)
	oldOrphan := filepath.Join(h.paths.FilesDir, "old_orphan_id")
	if err := os.WriteFile(oldOrphan, []byte("orphaned content"), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(oldOrphan, oldTime, oldTime)

	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.OrphansCleaned != 1 {
		t.Fatalf("expected 1 orphan file cleaned, got %d", stats.OrphansCleaned)
	}

	// Fresh file within grace period must remain
	if _, err := os.Stat(freshOrphan); err != nil {
		t.Fatalf("fresh file within grace period must not be removed: %v", err)
	}

	// Old orphan must be removed
	if _, err := os.Stat(oldOrphan); !os.IsNotExist(err) {
		t.Fatalf("old orphan file must be removed")
	}
}

func TestResilienceWhenPhysicalFileAlreadyMissing(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	// Create expired room with file
	rm, err := h.roomStore.Create(ctx, -time.Minute, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("missing test")
	up, err := h.fileStore.StreamUpload(ctx, rm.ID, "missing.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Manually delete file from disk before cleanup runs
	_ = os.Remove(filepath.Join(h.paths.FilesDir, up.StorageID))

	// Cleanup should succeed without errors
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("cleanup must succeed even if physical file missing: %v", err)
	}
	if stats.RoomsCleaned != 1 {
		t.Fatalf("expected 1 room cleaned, got %d", stats.RoomsCleaned)
	}
	if stats.Errors != 0 {
		t.Fatalf("expected 0 errors, got %d", stats.Errors)
	}
}

func TestBatchLimitRespected(t *testing.T) {
	opts := DefaultOptions()
	opts.BatchSize = 2
	h := setupTestHarness(t, opts)
	ctx := context.Background()

	// Create 5 expired rooms
	for i := 0; i < 5; i++ {
		_, err := h.roomStore.Create(ctx, -time.Minute, 10<<20, 2<<20, 10, "")
		if err != nil {
			t.Fatal(err)
		}
	}

	// First pass should clean exactly BatchSize (2) rooms
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 2 {
		t.Fatalf("expected 2 rooms cleaned according to batch size, got %d", stats.RoomsCleaned)
	}

	// Second pass cleans another 2 rooms
	stats, err = h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 2 {
		t.Fatalf("expected 2 rooms cleaned in second pass, got %d", stats.RoomsCleaned)
	}

	// Third pass cleans remaining 1 room
	stats, err = h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 1 {
		t.Fatalf("expected 1 room cleaned in final pass, got %d", stats.RoomsCleaned)
	}
}

func TestConcurrentUploadAndCleanup(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	// Create active room and expired room
	activeRoom, err := h.roomStore.Create(ctx, time.Hour, 100<<20, 10<<20, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.roomStore.Create(ctx, -time.Minute, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	// Concurrently upload files to active room while cleanup is running
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := bytes.Repeat([]byte(fmt.Sprintf("data-%d", idx)), 1024)
			_, _ = h.fileStore.StreamUpload(ctx, activeRoom.ID, fmt.Sprintf("file-%d.txt", idx), "text/plain", bytes.NewReader(data), int64(len(data)), 10<<20, 100<<20, 100)
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = h.worker.RunOnce(ctx)
	}()

	wg.Wait()

	// Verify active room files are intact
	files, err := h.fileStore.ListReadyFiles(ctx, activeRoom.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("expected 5 files in active room, got %d", len(files))
	}
}

func TestGracefulContextCancellation(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce with canceled context should return cleanly: %v", err)
	}
	if stats.RoomsCleaned != 0 {
		t.Fatalf("expected 0 rooms cleaned when canceled, got %d", stats.RoomsCleaned)
	}
}

func TestImmediateClosedRoomCleanup(t *testing.T) {
	opts := DefaultOptions()
	opts.ClosedRoomRetention = 0 // Immediate cleanup
	h := setupTestHarness(t, opts)
	ctx := context.Background()

	rm, err := h.roomStore.Create(ctx, time.Hour, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("immediate cleanup content")
	_, err = h.fileStore.StreamUpload(ctx, rm.ID, "test.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	if err := h.roomStore.Close(ctx, rm.CreatorToken); err != nil {
		t.Fatal(err)
	}

	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 1 || stats.FilesDeleted != 1 {
		t.Fatalf("expected 1 room and 1 file cleaned immediately, got stats: %+v", stats)
	}
}

func TestCleanRoomInvalidStorageIDIgnored(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	// Create expired room
	created, err := h.roomStore.Create(ctx, -time.Minute, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	// Insert a malicious path traversal storage_id directly into database
	nowStr := time.Now().UTC().Format(time.RFC3339)
	_, err = h.db.Exec(`
		INSERT INTO files (id, room_id, storage_id, original_filename, size_bytes, content_type, status, created_at, completed_at)
		VALUES ('f_malicious', ?, '../../../../outside.txt', 'evil.txt', 100, 'text/plain', 'ready', ?, ?);
	`, created.ID, nowStr, nowStr)
	if err != nil {
		t.Fatal(err)
	}

	// Run cleanup
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("cleanup must not crash on invalid storage ID: %v", err)
	}
	if stats.RoomsCleaned != 1 {
		t.Fatalf("expected 1 room cleaned, got %d", stats.RoomsCleaned)
	}
}

func TestCascadingShareDeletion(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	// Create expired room with file and share
	rm, err := h.roomStore.Create(ctx, -time.Minute, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("Cascade share test")
	uploaded, err := h.fileStore.StreamUpload(ctx, rm.ID, "share.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, time.Now().Add(time.Hour), 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}

	var shareCount int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM shares WHERE room_id = ?", rm.ID).Scan(&shareCount); err != nil {
		t.Fatal(err)
	}
	if shareCount != 1 {
		t.Fatalf("expected 1 share before cleanup, got %d", shareCount)
	}

	// Run cleanup
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RoomsCleaned != 1 {
		t.Fatalf("expected 1 room cleaned, got %d", stats.RoomsCleaned)
	}

	// Verify share record deleted via cascade
	if err := h.db.QueryRow("SELECT COUNT(*) FROM shares WHERE room_id = ?", rm.ID).Scan(&shareCount); err != nil {
		t.Fatal(err)
	}
	if shareCount != 0 {
		t.Fatalf("expected 0 shares after room cascade, got %d", shareCount)
	}
}

func TestExpiredAndRevokedShareSweep(t *testing.T) {
	h := setupTestHarness(t, DefaultOptions())
	ctx := context.Background()

	// Active room
	rm, err := h.roomStore.Create(ctx, time.Hour, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("Sweep share test")
	uploaded, err := h.fileStore.StreamUpload(ctx, rm.ID, "sweep.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Share 1: Expired
	sh1, _, err := h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	pastTime := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := h.db.Exec("UPDATE shares SET expires_at = ? WHERE id = ?", pastTime, sh1.ID); err != nil {
		t.Fatal(err)
	}

	// Share 2: Revoked
	sh2, _, err := h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.shareStore.RevokeShare(ctx, rm.ID, sh2.ID); err != nil {
		t.Fatal(err)
	}

	// Share 3: Active
	_, _, err = h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Run cleanup
	stats, err := h.worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SharesCleaned != 2 {
		t.Fatalf("expected 2 shares cleaned (1 expired + 1 revoked), got %d", stats.SharesCleaned)
	}

	// Active share should still exist
	var remainingShares int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM shares WHERE room_id = ?", rm.ID).Scan(&remainingShares); err != nil {
		t.Fatal(err)
	}
	if remainingShares != 1 {
		t.Fatalf("expected 1 active share remaining, got %d", remainingShares)
	}
}
