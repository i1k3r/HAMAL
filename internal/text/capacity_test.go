package text

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Client messages own the room capacity; system messages rotate and never block user text.
func TestTextCapacityEvictsSystemMessagesOnly(t *testing.T) {
	ctx := context.Background()
	roomStore, textStore := setupTestDB(t)
	rm, err := roomStore.Create(ctx, time.Hour, 10<<20, 5<<20, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	const limit = 4

	// System messages alone never exceed the cap and never fail.
	for i := 0; i < 10; i++ {
		if _, err := textStore.CreateServerText(ctx, rm.ID, "event "+string(rune('a'+i)), limit); err != nil {
			t.Fatalf("server text %d: %v", i, err)
		}
	}
	texts, err := textStore.ListRoomTexts(ctx, rm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != limit || texts[0].Content != "event g" || texts[limit-1].Content != "event j" {
		t.Fatalf("expected the newest %d system messages, got %+v", limit, texts)
	}

	// Client messages displace the oldest system messages...
	for i := 0; i < limit; i++ {
		if _, err := textStore.CreateClientText(ctx, rm.ID, "sess", "user", 65536, limit); err != nil {
			t.Fatalf("client text %d should evict a system message: %v", i, err)
		}
	}
	// ...and only client messages can make the room full.
	if _, err := textStore.CreateClientText(ctx, rm.ID, "sess", "overflow", 65536, limit); !errors.Is(err, ErrTextLimitReached) {
		t.Fatalf("expected ErrTextLimitReached, got %v", err)
	}
	if _, err := textStore.CreateServerText(ctx, rm.ID, "late event", limit); !errors.Is(err, ErrTextLimitReached) {
		t.Fatalf("a system message must not displace user text, got %v", err)
	}

	texts, err = textStore.ListRoomTexts(ctx, rm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != limit {
		t.Fatalf("expected %d texts, got %d", limit, len(texts))
	}
	for _, txt := range texts {
		if txt.SenderType != "client" {
			t.Fatalf("user text was displaced by %q", txt.Content)
		}
	}
}

func TestTextJSONOmitsSenderSessionID(t *testing.T) {
	sess := "cs_private"
	encoded, err := json.Marshal(Text{ID: "t1", SenderType: "client", SenderSessionID: &sess, Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "cs_private") || strings.Contains(string(encoded), "sender_session_id") {
		t.Fatalf("sender session id must not be serialised: %s", encoded)
	}
}
