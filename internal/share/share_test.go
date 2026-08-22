package share

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/i1k3r/HAMAL/internal/database"
	"github.com/i1k3r/HAMAL/internal/file"
	"github.com/i1k3r/HAMAL/internal/room"
	"github.com/i1k3r/HAMAL/internal/storage"
)

type shareTestHarness struct {
	db         *sql.DB
	paths      storage.Paths
	roomStore  *room.Store
	fileStore  *file.Store
	shareStore *Store
	secret     string
}

func setupShareHarness(t *testing.T) *shareTestHarness {
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

	secret := "test-secret-at-least-32-bytes-long-for-shares"
	roomStore := room.NewStore(db, secret)
	quotaMgr := file.NewQuotaManager()
	fileStore := file.NewStore(db, paths, quotaMgr)
	shareStore := NewStore(db, secret)

	return &shareTestHarness{
		db:         db,
		paths:      paths,
		roomStore:  roomStore,
		fileStore:  fileStore,
		shareStore: shareStore,
		secret:     secret,
	}
}

func TestTokenGenerationAndHashing(t *testing.T) {
	token1, err := GenerateShareToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token1, "gsh_") {
		t.Fatalf("expected prefix gsh_, got %s", token1)
	}
	if len(token1) != 68 {
		t.Fatalf("expected length 68, got %d", len(token1))
	}

	token2, err := GenerateShareToken()
	if err != nil {
		t.Fatal(err)
	}
	if token1 == token2 {
		t.Fatal("expected unique random tokens")
	}

	secret := "secret-123456789012345678901234567890"
	hash1 := HashShareToken(secret, token1)
	hash2 := HashShareToken(secret, token1)
	if hash1 != hash2 {
		t.Fatal("HMAC hash must be deterministic")
	}
	if len(hash1) != 64 {
		t.Fatalf("expected 64 hex chars for SHA-256 HMAC, got %d", len(hash1))
	}
}

func TestCreateShareAndGetByToken(t *testing.T) {
	h := setupShareHarness(t)
	ctx := context.Background()

	rm, err := h.roomStore.Create(ctx, 2*time.Hour, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("Global share file content")
	uploaded, err := h.fileStore.StreamUpload(ctx, rm.ID, "report.pdf", "application/pdf", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	sh, token, err := h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	if sh.Status != "active" {
		t.Fatalf("expected active status, got %s", sh.Status)
	}

	// Retrieve by token
	retrievedShare, retrievedFile, err := h.shareStore.GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("GetByToken failed: %v", err)
	}
	if retrievedShare.ID != sh.ID {
		t.Fatalf("expected share id %s, got %s", sh.ID, retrievedShare.ID)
	}
	if retrievedFile.ID != uploaded.ID {
		t.Fatalf("expected file id %s, got %s", uploaded.ID, retrievedFile.ID)
	}
	if retrievedFile.OriginalFilename != "report.pdf" {
		t.Fatalf("expected report.pdf, got %s", retrievedFile.OriginalFilename)
	}
}

func TestShareExpirationBoundByRoom(t *testing.T) {
	h := setupShareHarness(t)
	ctx := context.Background()

	// Room expires in 15 minutes
	rm, err := h.roomStore.Create(ctx, 15*time.Minute, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("Bounded expiration test")
	uploaded, err := h.fileStore.StreamUpload(ctx, rm.ID, "data.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Request 2 hour share TTL
	sh, _, err := h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, 2*time.Hour, rm.ExpiresAt, 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Share expiration must not exceed room expiration (with 1s leeway for clock ticks)
	if sh.ExpiresAt.After(rm.ExpiresAt.Add(time.Second)) {
		t.Fatalf("share expiration %v exceeded room expiration %v", sh.ExpiresAt, rm.ExpiresAt)
	}
}

func TestShareRevocation(t *testing.T) {
	h := setupShareHarness(t)
	ctx := context.Background()

	rm, err := h.roomStore.Create(ctx, time.Hour, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("Revocation test")
	uploaded, err := h.fileStore.StreamUpload(ctx, rm.ID, "revoke.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	sh, token, err := h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Revoke share
	if err := h.shareStore.RevokeShare(ctx, rm.ID, sh.ID); err != nil {
		t.Fatalf("RevokeShare failed: %v", err)
	}

	// Attempting to access revoked share must fail with ErrShareRevoked
	_, _, err = h.shareStore.GetByToken(ctx, token)
	if err != ErrShareRevoked {
		t.Fatalf("expected ErrShareRevoked, got %v", err)
	}

	// Underlying room file must remain intact
	files, err := h.fileStore.ListReadyFiles(ctx, rm.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("room file must remain accessible after share revocation: %v", err)
	}
}

func TestRoomCloseInvalidatesShare(t *testing.T) {
	h := setupShareHarness(t)
	ctx := context.Background()

	rm, err := h.roomStore.Create(ctx, time.Hour, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("Room close test")
	uploaded, err := h.fileStore.StreamUpload(ctx, rm.ID, "closed.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	_, token, err := h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Close room
	if err := h.roomStore.Close(ctx, rm.CreatorToken); err != nil {
		t.Fatal(err)
	}

	// Share access should fail with ErrRoomInactive
	_, _, err = h.shareStore.GetByToken(ctx, token)
	if err != ErrRoomInactive {
		t.Fatalf("expected ErrRoomInactive, got %v", err)
	}
}

func TestMaxSharesPerRoomEnforced(t *testing.T) {
	h := setupShareHarness(t)
	ctx := context.Background()

	rm, err := h.roomStore.Create(ctx, time.Hour, 10<<20, 2<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("Max shares test")
	uploaded, err := h.fileStore.StreamUpload(ctx, rm.ID, "limit.txt", "text/plain", bytes.NewReader(content), int64(len(content)), 2<<20, 10<<20, 10)
	if err != nil {
		t.Fatal(err)
	}

	maxShares := 2
	for i := 0; i < maxShares; i++ {
		_, _, err := h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, maxShares)
		if err != nil {
			t.Fatalf("expected share %d to succeed: %v", i+1, err)
		}
	}

	// 3rd share must be rejected
	_, _, err = h.shareStore.CreateShare(ctx, rm.ID, uploaded.ID, time.Hour, rm.ExpiresAt, 24*time.Hour, maxShares)
	if err != ErrShareLimitReached {
		t.Fatalf("expected ErrShareLimitReached, got %v", err)
	}
}

func TestInvalidShareTokens(t *testing.T) {
	h := setupShareHarness(t)
	ctx := context.Background()

	tests := []string{
		"",
		"gsh_short",
		"c_12345678901234567890123456789012",
		"r_12345678901234567890123456789012",
		"gsh_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	}

	for _, token := range tests {
		_, _, err := h.shareStore.GetByToken(ctx, token)
		if err != ErrInvalidToken && err != ErrShareNotFound {
			t.Fatalf("token %q expected invalid/not found error, got %v", token, err)
		}
	}
}
