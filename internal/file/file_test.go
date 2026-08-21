package file

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/i1k3r/lan-drop/internal/database"
	"github.com/i1k3r/lan-drop/internal/room"
	"github.com/i1k3r/lan-drop/internal/storage"
)

func setupTestStore(t *testing.T) (*Store, *room.Store, string, storage.Paths) {
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

	secret := "test-secret-must-be-at-least-32-bytes-long"
	roomStore := room.NewStore(db, secret)
	quotaManager := NewQuotaManager()
	fileStore := NewStore(db, paths, quotaManager)

	created, err := roomStore.Create(context.Background(), time.Hour, 10<<20, 2<<20, 5, "") // 10MB room, 2MB file, 5 files max
	if err != nil {
		t.Fatal(err)
	}

	return fileStore, roomStore, created.ID, paths
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal_file.pdf", "normal_file.pdf"},
		{"../../../etc/passwd", "passwd"},
		{"..\\..\\Windows\\System32\\cmd.exe", "cmd.exe"},
		{"<script>alert('xss')</script>.jpg", "<script>alert('xss')<script>.jpg"},
		{"\x00evil\x1fnull.png", "_evil_null.png"},
		{"", "unnamed_file"},
		{".", "unnamed_file"},
		{"..", "unnamed_file"},
		{"   ", "unnamed_file"},
		{"my document (v1.2).docx", "my document (v1.2).docx"},
		{"Sözleşme_2026_İlker.pdf", "Sözleşme_2026_İlker.pdf"},
		{"Antalya Şubesi – Fotoğraf.jpg", "Antalya Şubesi – Fotoğraf.jpg"},
		{"çalışma notları.txt", "çalışma notları.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if strings.Contains(got, "/") || strings.Contains(got, "\\") {
				t.Fatalf("sanitized filename must not contain path separators: %q", got)
			}
			if len(got) == 0 {
				t.Fatal("sanitized filename must not be empty")
			}
		})
	}
}

func TestSanitizeContentDisposition(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		hasUTF8 string
		noCRLF  bool
	}{
		{
			name:    "simple ascii",
			input:   "document.pdf",
			hasUTF8: "document.pdf",
			noCRLF:  true,
		},
		{
			name:    "turkish characters",
			input:   "Sözleşme_2026_İlker.pdf",
			hasUTF8: "S%C3%B6zle%C5%9Fme_2026_%C4%B0lker.pdf",
			noCRLF:  true,
		},
		{
			name:    "turkish with spaces and dashes",
			input:   "Antalya Şubesi – Fotoğraf.jpg",
			hasUTF8: "Antalya%20%C5%9Eubesi%20%E2%80%93%20Foto%C4%9Fraf.jpg",
			noCRLF:  true,
		},
		{
			name:    "turkish notes with spaces",
			input:   "çalışma notları.txt",
			hasUTF8: "%C3%A7al%C4%B1%C5%9Fma%20notlar%C4%B1.txt",
			noCRLF:  true,
		},
		{
			name:    "CRLF injection payload",
			input:   "evil\r\nSet-Cookie: sessionId=123\r\n.pdf",
			hasUTF8: "evil%0D%0ASet-Cookie:%20sessionId=123%0D%0A.pdf",
			noCRLF:  true,
		},
		{
			name:    "quotes and semicolons injection",
			input:   `bad"; filename="injected.exe`,
			hasUTF8: "bad%22%3B%20filename=%22injected.exe",
			noCRLF:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disposition := SanitizeContentDisposition(tt.input)
			if !strings.HasPrefix(disposition, "attachment; filename=\"") {
				t.Fatalf("expected attachment prefix, got %q", disposition)
			}
			if !strings.Contains(disposition, "filename*=UTF-8''") {
				t.Fatalf("expected UTF-8 filename parameter, got %q", disposition)
			}
			// Extract ascii filename part between quotes
			firstQuote := strings.Index(disposition, "\"")
			secondQuote := strings.Index(disposition[firstQuote+1:], "\"")
			if secondQuote == -1 {
				t.Fatalf("malformed quotes in disposition: %q", disposition)
			}
			asciiPart := disposition[firstQuote+1 : firstQuote+1+secondQuote]
			if strings.Contains(asciiPart, "\r") || strings.Contains(asciiPart, "\n") || strings.Contains(asciiPart, "\"") || strings.Contains(asciiPart, ";") {
				t.Fatalf("ascii filename contains unsafe characters: %q", asciiPart)
			}
			if !strings.Contains(disposition, tt.hasUTF8) {
				t.Fatalf("expected disposition to contain UTF-8 encoded string %q, got %q", tt.hasUTF8, disposition)
			}
		})
	}
}

func TestQuotaManagerConcurrentReservations(t *testing.T) {
	qm := NewQuotaManager()
	roomID := "room-quota-test"
	maxRoomSize := int64(1000)

	// Acquire 600 bytes
	res1, err := qm.Acquire(roomID, 600, 0, maxRoomSize)
	if err != nil {
		t.Fatalf("failed to acquire res1: %v", err)
	}

	// Attempt to acquire another 600 bytes -> must fail because 600+600 > 1000
	_, err = qm.Acquire(roomID, 600, 0, maxRoomSize)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}

	// Release res1 and acquire 800 bytes -> must succeed
	qm.Release(res1)
	res2, err := qm.Acquire(roomID, 800, 0, maxRoomSize)
	if err != nil {
		t.Fatalf("failed to acquire res2 after release: %v", err)
	}

	// Concurrent race test: 10 goroutines attempting to acquire remaining capacity
	var wg sync.WaitGroup
	var acquiredCount int
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := qm.Acquire(roomID, 300, 800, maxRoomSize) // 800 finalized + 300 > 1000
			if err == nil {
				mu.Lock()
				acquiredCount++
				mu.Unlock()
				qm.Release(id)
			}
		}()
	}
	wg.Wait()

	if acquiredCount != 0 {
		t.Fatalf("expected 0 concurrent acquisitions beyond capacity, got %d", acquiredCount)
	}
	qm.Release(res2)
}

func TestStreamUploadSuccessAndListing(t *testing.T) {
	store, _, roomID, paths := setupTestStore(t)
	ctx := context.Background()

	content := []byte("Hello, LAN-Drop file upload!")
	file, err := store.StreamUpload(
		ctx,
		roomID,
		"test_upload.txt",
		"text/plain",
		bytes.NewReader(content),
		int64(len(content)),
		2<<20,
		10<<20,
		5,
	)
	if err != nil {
		t.Fatalf("StreamUpload failed: %v", err)
	}

	if file.ID == "" || !strings.HasPrefix(file.ID, "f_") {
		t.Fatalf("unexpected file ID: %q", file.ID)
	}
	if file.SizeBytes != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), file.SizeBytes)
	}

	// Verify file exists in /data/files/<storage_id>
	finalPath := filepath.Join(paths.FilesDir, file.StorageID)
	savedBytes, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("failed to read saved file from %s: %v", finalPath, err)
	}
	if !bytes.Equal(savedBytes, content) {
		t.Fatal("saved content does not match original upload")
	}

	// Verify file is returned in ListReadyFiles
	list, err := store.ListReadyFiles(ctx, roomID)
	if err != nil {
		t.Fatalf("ListReadyFiles failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 file, got %d", len(list))
	}
	if list[0].ID != file.ID || list[0].OriginalFilename != "test_upload.txt" {
		t.Fatalf("unexpected listed file: %+v", list[0])
	}

	// Test GetReadyFile
	readyFile, err := store.GetReadyFile(ctx, roomID, file.ID)
	if err != nil {
		t.Fatalf("GetReadyFile failed: %v", err)
	}
	if readyFile.ID != file.ID || readyFile.StorageID != file.StorageID {
		t.Fatalf("unexpected ready file: %+v", readyFile)
	}

	// Test GetReadyFile with invalid room ID
	_, err = store.GetReadyFile(ctx, "wrong-room-id", file.ID)
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("expected ErrFileNotFound for wrong room, got %v", err)
	}

	// Test OpenStorageFile
	f, err := store.OpenStorageFile(file.StorageID)
	if err != nil {
		t.Fatalf("OpenStorageFile failed: %v", err)
	}
	_ = f.Close()
}

func TestStreamUploadEmptyFileRejected(t *testing.T) {
	store, _, roomID, paths := setupTestStore(t)
	ctx := context.Background()

	_, err := store.StreamUpload(
		ctx,
		roomID,
		"empty.txt",
		"text/plain",
		bytes.NewReader([]byte{}),
		0,
		2<<20,
		10<<20,
		5,
	)
	if !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("expected ErrEmptyFile, got %v", err)
	}

	// Verify no staging files left in staging directory
	entries, err := os.ReadDir(paths.StagingDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 staging files, found %d", len(entries))
	}
}

func TestStreamUploadExceedsMaxFileSize(t *testing.T) {
	store, _, roomID, paths := setupTestStore(t)
	ctx := context.Background()

	largeContent := make([]byte, 1024)
	maxFileSize := int64(512) // Limit to 512 bytes

	_, err := store.StreamUpload(
		ctx,
		roomID,
		"oversized.dat",
		"application/octet-stream",
		bytes.NewReader(largeContent),
		int64(len(largeContent)),
		maxFileSize,
		10<<20,
		5,
	)
	if !errors.Is(err, ErrFileTooLarge) && !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected size limit error, got %v", err)
	}

	// Verify staging file was cleaned up
	entries, _ := os.ReadDir(paths.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("expected staging dir to be clean, found %d files", len(entries))
	}
}

func TestStreamUploadRoomQuotaExceeded(t *testing.T) {
	store, _, roomID, _ := setupTestStore(t)
	ctx := context.Background()

	// Upload first file (600 bytes in 1000-byte room)
	content1 := make([]byte, 600)
	_, err := store.StreamUpload(
		ctx,
		roomID,
		"part1.dat",
		"application/octet-stream",
		bytes.NewReader(content1),
		600,
		1000,
		1000,
		5,
	)
	if err != nil {
		t.Fatalf("first upload failed: %v", err)
	}

	// Upload second file (600 bytes) -> must exceed remaining 400 bytes quota
	content2 := make([]byte, 600)
	_, err = store.StreamUpload(
		ctx,
		roomID,
		"part2.dat",
		"application/octet-stream",
		bytes.NewReader(content2),
		600,
		1000,
		1000,
		5,
	)
	if !errors.Is(err, ErrQuotaExceeded) && !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected quota error for second upload, got %v", err)
	}
}

func TestStreamUploadRollbackOnDatabaseFailure(t *testing.T) {
	store, _, _, paths := setupTestStore(t)
	ctx := context.Background()

	// Use an invalid room ID that violates foreign key constraint in SQLite
	nonExistentRoomID := "non-existent-room-id"

	content := []byte("Rollback test file content")
	_, err := store.StreamUpload(
		ctx,
		nonExistentRoomID,
		"rollback_test.txt",
		"text/plain",
		bytes.NewReader(content),
		int64(len(content)),
		2<<20,
		10<<20,
		5,
	)
	if err == nil {
		t.Fatal("expected error due to foreign key violation, got nil")
	}

	// Verify no orphaned finalized file remains in /data/files
	filesEntries, err := os.ReadDir(paths.FilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(filesEntries) != 0 {
		t.Fatalf("expected 0 files in finalized directory after rollback, found %d", len(filesEntries))
	}

	// Verify staging directory is also clean
	stagingEntries, err := os.ReadDir(paths.StagingDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(stagingEntries) != 0 {
		t.Fatalf("expected 0 files in staging directory, found %d", len(stagingEntries))
	}
}
