package text

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/i1k3r/HAMAL/internal/database"
	"github.com/i1k3r/HAMAL/internal/room"
)

func setupTestDB(t *testing.T) (*room.Store, *Store) {
	t.Helper()
	tempDir := t.TempDir()
	db, err := database.Open(tempDir + "/test.db")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	roomStore := room.NewStore(db, "test-secret-must-be-at-least-32-bytes-long!")
	textStore := NewStore(db)
	return roomStore, textStore
}

func TestCreateAndListTexts(t *testing.T) {
	ctx := context.Background()
	roomStore, textStore := setupTestDB(t)

	rm, err := roomStore.Create(ctx, time.Hour, 10<<20, 5<<20, 10, "")
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	// 1. Initially empty
	texts, err := textStore.ListRoomTexts(ctx, rm.ID)
	if err != nil {
		t.Fatalf("unexpected error listing texts: %v", err)
	}
	if len(texts) != 0 {
		t.Fatalf("expected 0 texts, got %d", len(texts))
	}

	// 2. Add SERVER message
	sMsg, err := textStore.CreateServerText(ctx, rm.ID, "Room created", 100)
	if err != nil {
		t.Fatalf("failed to create server text: %v", err)
	}
	if sMsg.ID == "" || sMsg.SenderType != "server" || sMsg.SenderSessionID != nil {
		t.Fatalf("unexpected server message: %+v", sMsg)
	}

	// 3. Add CLIENT message from session A
	sessA := "session-aaa-1234"
	cMsg1, err := textStore.CreateClientText(ctx, rm.ID, sessA, "sudo journalctl -u nginx", 65536, 100)
	if err != nil {
		t.Fatalf("failed to create client text 1: %v", err)
	}
	if cMsg1.ID == "" || cMsg1.SenderType != "client" || cMsg1.SenderSessionID == nil || *cMsg1.SenderSessionID != sessA {
		t.Fatalf("unexpected client message 1: %+v", cMsg1)
	}

	// 4. Add CLIENT message from session B
	sessB := "session-bbb-5678"
	cMsg2, err := textStore.CreateClientText(ctx, rm.ID, sessB, "https://example.com/log.txt\nLine 2 output", 65536, 100)
	if err != nil {
		t.Fatalf("failed to create client text 2: %v", err)
	}
	if cMsg2.ID == "" || cMsg2.SenderType != "client" || cMsg2.SenderSessionID == nil || *cMsg2.SenderSessionID != sessB {
		t.Fatalf("unexpected client message 2: %+v", cMsg2)
	}

	// 5. List texts in room
	texts, err = textStore.ListRoomTexts(ctx, rm.ID)
	if err != nil {
		t.Fatalf("failed to list texts: %v", err)
	}
	if len(texts) != 3 {
		t.Fatalf("expected 3 texts, got %d", len(texts))
	}
	if texts[0].ID != sMsg.ID || texts[1].ID != cMsg1.ID || texts[2].ID != cMsg2.ID {
		t.Fatalf("order mismatch: %+v", texts)
	}

	// 6. Count texts
	count, err := textStore.CountRoomTexts(ctx, rm.ID)
	if err != nil {
		t.Fatalf("failed to count texts: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestTextValidationAndLimits(t *testing.T) {
	ctx := context.Background()
	roomStore, textStore := setupTestDB(t)

	rm, err := roomStore.Create(ctx, time.Hour, 10<<20, 5<<20, 10, "")
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	// 1. Empty text rejected
	_, err = textStore.CreateClientText(ctx, rm.ID, "sess", "", 65536, 100)
	if err != ErrTextEmpty {
		t.Fatalf("expected ErrTextEmpty, got %v", err)
	}
	_, err = textStore.CreateClientText(ctx, rm.ID, "sess", "   \n\t  ", 65536, 100)
	if err != ErrTextEmpty {
		t.Fatalf("expected ErrTextEmpty for whitespace, got %v", err)
	}
	_, err = textStore.CreateServerText(ctx, rm.ID, "  ", 100)
	if err != ErrTextEmpty {
		t.Fatalf("expected ErrTextEmpty for server whitespace, got %v", err)
	}

	// 2. Oversized text rejected
	largeText := strings.Repeat("A", 1000)
	_, err = textStore.CreateClientText(ctx, rm.ID, "sess", largeText, 500, 100)
	if err != ErrTextTooLarge {
		t.Fatalf("expected ErrTextTooLarge, got %v", err)
	}

	// 3. Max texts limit
	_, err = textStore.CreateClientText(ctx, rm.ID, "sess", "msg 1", 65536, 2)
	if err != nil {
		t.Fatalf("failed msg 1: %v", err)
	}
	_, err = textStore.CreateClientText(ctx, rm.ID, "sess", "msg 2", 65536, 2)
	if err != nil {
		t.Fatalf("failed msg 2: %v", err)
	}
	_, err = textStore.CreateClientText(ctx, rm.ID, "sess", "msg 3", 65536, 2)
	if err != ErrTextLimitReached {
		t.Fatalf("expected ErrTextLimitReached, got %v", err)
	}
}

func TestTextRoomIsolationAndCascade(t *testing.T) {
	ctx := context.Background()
	roomStore, textStore := setupTestDB(t)

	rm1, err := roomStore.Create(ctx, time.Hour, 10<<20, 5<<20, 10, "")
	if err != nil {
		t.Fatalf("failed to create room 1: %v", err)
	}
	rm2, err := roomStore.Create(ctx, time.Hour, 10<<20, 5<<20, 10, "")
	if err != nil {
		t.Fatalf("failed to create room 2: %v", err)
	}

	_, err = textStore.CreateClientText(ctx, rm1.ID, "sess1", "Secret room 1 note", 65536, 100)
	if err != nil {
		t.Fatalf("failed to create text in rm1: %v", err)
	}
	_, err = textStore.CreateClientText(ctx, rm2.ID, "sess2", "Secret room 2 note", 65536, 100)
	if err != nil {
		t.Fatalf("failed to create text in rm2: %v", err)
	}

	// Room 1 should only see room 1 text
	texts1, err := textStore.ListRoomTexts(ctx, rm1.ID)
	if err != nil || len(texts1) != 1 || texts1[0].Content != "Secret room 1 note" {
		t.Fatalf("room isolation failure for room 1: %+v", texts1)
	}

	// Room 2 should only see room 2 text
	texts2, err := textStore.ListRoomTexts(ctx, rm2.ID)
	if err != nil || len(texts2) != 1 || texts2[0].Content != "Secret room 2 note" {
		t.Fatalf("room isolation failure for room 2: %+v", texts2)
	}

	// Closing room 1 cascades deletion of texts
	if err := roomStore.CloseByRoomID(ctx, rm1.ID); err != nil {
		t.Fatalf("failed to close room 1: %v", err)
	}

	// Creating in closed room fails with ErrRoomInactive
	_, err = textStore.CreateClientText(ctx, rm1.ID, "sess1", "Late message", 65536, 100)
	if err != ErrRoomInactive {
		t.Fatalf("expected ErrRoomInactive for closed room, got %v", err)
	}
}

