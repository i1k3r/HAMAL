package file

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/i1k3r/HAMAL/internal/database"
	"github.com/i1k3r/HAMAL/internal/room"
	"github.com/i1k3r/HAMAL/internal/storage"
)

// gatedReader yields head, then blocks until gate is closed, then yields tail.
// It models an upload that is still in flight while a sibling upload completes.
type gatedReader struct {
	head io.Reader
	tail io.Reader
	gate <-chan struct{}
}

func (g *gatedReader) Read(p []byte) (int, error) {
	n, err := g.head.Read(p)
	if n > 0 || err != io.EOF {
		return n, err
	}
	<-g.gate
	return g.tail.Read(p)
}

func newQuotaTestStore(t *testing.T, opts StoreOptions) (*Store, *room.Store, *QuotaManager) {
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

	qm := NewQuotaManager()
	return NewStore(db, paths, qm, opts), room.NewStore(db, "test-secret-must-be-at-least-32-bytes-long"), qm
}

func waitForReservation(t *testing.T, qm *QuotaManager, roomID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for qm.GetActiveReserved(roomID) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("in-flight upload never acquired a reservation")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// An upload that started before a sibling finished must still count the sibling's committed bytes.
func TestInFlightUploadCountsSiblingCommittedBytes(t *testing.T) {
	const kb = int64(1024)
	const mb = 1024 * kb
	store, rooms, qm := newQuotaTestStore(t, StoreOptions{})
	ctx := context.Background()

	maxRoom := 8 * mb
	r, err := rooms.Create(ctx, time.Hour, maxRoom, maxRoom, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	gate := make(chan struct{})
	slowDone := make(chan error, 1)
	go func() {
		reader := &gatedReader{
			head: bytes.NewReader(bytes.Repeat([]byte("s"), int(kb))),
			tail: bytes.NewReader(bytes.Repeat([]byte("s"), int(6*mb))),
			gate: gate,
		}
		_, err := store.StreamUpload(ctx, r.ID, "slow.bin", "application/octet-stream", reader, 0, maxRoom, maxRoom, 10)
		slowDone <- err
	}()
	waitForReservation(t, qm, r.ID)

	// The sibling completes 5 MiB while the slow upload has only sent 1 KB.
	fast := bytes.Repeat([]byte("f"), int(5*mb))
	if _, err := store.StreamUpload(ctx, r.ID, "fast.bin", "application/octet-stream", bytes.NewReader(fast), int64(len(fast)), maxRoom, maxRoom, 10); err != nil {
		t.Fatalf("fast upload should fit in the room: %v", err)
	}

	close(gate)
	if err := <-slowDone; !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("slow upload must be rejected once the room is full, got %v", err)
	}

	usage, count, err := store.GetRoomUsageAndCount(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage > maxRoom || count != 1 {
		t.Fatalf("room quota exceeded: usage=%d (max %d), files=%d", usage, maxRoom, count)
	}
	if qm.GetActiveReserved(r.ID) != 0 || qm.GetTotalActiveReserved() != 0 {
		t.Fatalf("reservations leaked: room=%d total=%d", qm.GetActiveReserved(r.ID), qm.GetTotalActiveReserved())
	}
}

// Many uploads opened at once and completed one after another must not multiply the room quota.
func TestStaggeredUploadsCannotMultiplyRoomQuota(t *testing.T) {
	const mb = int64(1024 * 1024)
	store, rooms, qm := newQuotaTestStore(t, StoreOptions{})
	ctx := context.Background()

	maxRoom := 8 * mb
	r, err := rooms.Create(ctx, time.Hour, maxRoom, maxRoom, 20, "")
	if err != nil {
		t.Fatal(err)
	}

	const uploads = 6
	gates := make([]chan struct{}, uploads)
	results := make([]chan error, uploads)
	for i := 0; i < uploads; i++ {
		gates[i] = make(chan struct{})
		results[i] = make(chan error, 1)
		go func(idx int) {
			reader := &gatedReader{
				head: bytes.NewReader([]byte("x")),
				tail: bytes.NewReader(bytes.Repeat([]byte("x"), int(6*mb))),
				gate: gates[idx],
			}
			_, err := store.StreamUpload(ctx, r.ID, "staggered.bin", "application/octet-stream", reader, 0, maxRoom, maxRoom, 20)
			results[idx] <- err
		}(i)
	}

	deadline := time.Now().Add(5 * time.Second)
	for qm.GetActiveFiles(r.ID) < uploads {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d uploads acquired a slot", qm.GetActiveFiles(r.ID), uploads)
		}
		time.Sleep(2 * time.Millisecond)
	}

	succeeded := 0
	for i := 0; i < uploads; i++ {
		close(gates[i])
		if err := <-results[i]; err == nil {
			succeeded++
		} else if !errors.Is(err, ErrQuotaExceeded) {
			t.Fatalf("upload %d failed with unexpected error: %v", i, err)
		}
	}

	usage, _, err := store.GetRoomUsageAndCount(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage > maxRoom {
		t.Fatalf("room quota multiplied by staggered uploads: usage=%d, max=%d", usage, maxRoom)
	}
	if succeeded != 1 {
		t.Fatalf("expected exactly one 6 MiB upload to fit in an 8 MiB room, got %d", succeeded)
	}
}

// The global storage cap must also hold when an upload in another room commits mid-transfer.
func TestInFlightUploadCountsGlobalCommittedBytes(t *testing.T) {
	const kb = int64(1024)
	const mb = 1024 * kb
	maxGlobal := 8 * mb
	store, rooms, qm := newQuotaTestStore(t, StoreOptions{MaxTotalStorage: maxGlobal})
	ctx := context.Background()

	maxRoom := 32 * mb
	roomA, err := rooms.Create(ctx, time.Hour, maxRoom, maxRoom, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	roomB, err := rooms.Create(ctx, time.Hour, maxRoom, maxRoom, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	gate := make(chan struct{})
	slowDone := make(chan error, 1)
	go func() {
		reader := &gatedReader{
			head: bytes.NewReader(bytes.Repeat([]byte("s"), int(kb))),
			tail: bytes.NewReader(bytes.Repeat([]byte("s"), int(6*mb))),
			gate: gate,
		}
		_, err := store.StreamUpload(ctx, roomA.ID, "slow.bin", "application/octet-stream", reader, 0, maxRoom, maxRoom, 10)
		slowDone <- err
	}()
	waitForReservation(t, qm, roomA.ID)

	fast := bytes.Repeat([]byte("f"), int(5*mb))
	if _, err := store.StreamUpload(ctx, roomB.ID, "fast.bin", "application/octet-stream", bytes.NewReader(fast), int64(len(fast)), maxRoom, maxRoom, 10); err != nil {
		t.Fatalf("fast upload should fit in the global quota: %v", err)
	}

	close(gate)
	if err := <-slowDone; !errors.Is(err, ErrGlobalStorageExceeded) {
		t.Fatalf("slow upload must be rejected once global storage is full, got %v", err)
	}

	total, err := store.GetTotalUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total > maxGlobal {
		t.Fatalf("global quota exceeded: usage=%d, max=%d", total, maxGlobal)
	}
}

// Free space is re-checked while streaming, not only before the first byte.
func TestMinFreeSpaceEnforcedDuringUpload(t *testing.T) {
	var calls atomic.Int64
	store, rooms, _ := newQuotaTestStore(t, StoreOptions{
		MinFreeSpace: 100 << 20,
		FreeSpaceFn: func(string) (uint64, error) {
			if calls.Add(1) <= 2 {
				return 500 << 20, nil
			}
			return 50 << 20, nil // disk filled up mid-transfer
		},
	})
	ctx := context.Background()

	r, err := rooms.Create(ctx, time.Hour, 64<<20, 64<<20, 5, "")
	if err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte("d"), 8<<20)
	_, err = store.StreamUpload(ctx, r.ID, "big.bin", "application/octet-stream", bytes.NewReader(payload), int64(len(payload)), 64<<20, 64<<20, 5)
	if !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("expected ErrInsufficientStorage once free space drops mid-upload, got %v", err)
	}

	usage, count, err := store.GetRoomUsageAndCount(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage != 0 || count != 0 {
		t.Fatalf("rejected upload must not be recorded: usage=%d files=%d", usage, count)
	}
}

func TestSanitizeContentType(t *testing.T) {
	cases := map[string]string{
		"text/plain":                      "text/plain",
		" Text/Plain ; Charset=UTF-8 ":    "text/plain; charset=UTF-8",
		"application/pdf; name=\"a.pdf\"": "application/pdf",
		"":                                "",
		"not a media type":                "",
		"text/plain; charset":             "",
		"application/" + strings.Repeat("x", 300): "",
	}
	for in, want := range cases {
		if got := SanitizeContentType(in); got != want {
			t.Errorf("SanitizeContentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStreamUploadBoundsStoredContentType(t *testing.T) {
	store, rooms, _ := newQuotaTestStore(t, StoreOptions{})
	ctx := context.Background()
	r, err := rooms.Create(ctx, time.Hour, 1<<20, 1<<20, 5, "")
	if err != nil {
		t.Fatal(err)
	}

	huge := "text/" + strings.Repeat("a", 1<<20)
	f, err := store.StreamUpload(ctx, r.ID, "note.txt", huge, strings.NewReader("hello world"), 11, 1<<20, 1<<20, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.ContentType) > maxContentTypeLength || !strings.HasPrefix(f.ContentType, "text/plain") {
		t.Fatalf("oversized declared content type must fall back to sniffing, got %d bytes: %.40q", len(f.ContentType), f.ContentType)
	}
}
