package room

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/i1k3r/HAMAL/internal/database"
)

func setupTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	secret := "test-server-secret-must-be-long-enough-32-bytes"
	return NewStore(db, secret), secret
}

func TestGenerateTokens(t *testing.T) {
	c1, p1, err := GenerateTokens()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c1, "c_") {
		t.Fatalf("expected creator token to start with c_, got %q", c1)
	}
	if !strings.HasPrefix(p1, "p_") {
		t.Fatalf("expected participant token to start with p_, got %q", p1)
	}
	if c1 == p1 {
		t.Fatal("creator and participant tokens must be distinct")
	}

	c2, p2, err := GenerateTokens()
	if err != nil {
		t.Fatal(err)
	}
	if c1 == c2 || p1 == p2 {
		t.Fatal("subsequent tokens must be randomly unique")
	}
}

func TestDeriveParticipantToken(t *testing.T) {
	creatorToken := "c_abcdef123456789012345678"
	p1 := DeriveParticipantToken(creatorToken)
	p2 := DeriveParticipantToken(creatorToken)

	if p1 != p2 {
		t.Fatal("derivation must be strictly deterministic")
	}
	if !strings.HasPrefix(p1, "p_") {
		t.Fatalf("expected prefix p_, got %q", p1)
	}
	if p1 == creatorToken {
		t.Fatal("derived participant token must differ from creator token")
	}
}

func TestHashToken(t *testing.T) {
	secret1 := "secret-one-012345678901234567890123456789"
	secret2 := "secret-two-012345678901234567890123456789"
	token := "p_somerandomtokenstring1234567890"

	h1 := HashToken(secret1, token)
	h2 := HashToken(secret1, token)
	if h1 != h2 {
		t.Fatal("same secret and token must produce identical hash")
	}

	h3 := HashToken(secret2, token)
	if h1 == h3 {
		t.Fatal("different secrets must produce different hashes")
	}

	h4 := HashToken(secret1, token+"other")
	if h1 == h4 {
		t.Fatal("different tokens must produce different hashes")
	}
}

func TestStoreCreateAndGetByToken(t *testing.T) {
	store, _ := setupTestStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, time.Hour, 10<<30, 2<<30, 100, "")
	if err != nil {
		t.Fatal(err)
	}

	if created.ID == "" {
		t.Fatal("expected non-empty room ID")
	}
	if created.TTLSeconds != 3600 {
		t.Fatalf("expected TTL 3600s, got %d", created.TTLSeconds)
	}
	if created.PinRequired {
		t.Fatal("expected pin_required to be false")
	}

	// Lookup via creator token
	cRoom, cRole, err := store.GetByToken(ctx, created.CreatorToken)
	if err != nil {
		t.Fatalf("lookup creator token failed: %v", err)
	}
	if cRole != RoleCreator {
		t.Fatalf("expected role %s, got %s", RoleCreator, cRole)
	}
	if cRoom.ID != created.ID {
		t.Fatalf("expected room ID %s, got %s", created.ID, cRoom.ID)
	}

	// Lookup via participant token
	pRoom, pRole, err := store.GetByToken(ctx, created.ParticipantToken)
	if err != nil {
		t.Fatalf("lookup participant token failed: %v", err)
	}
	if pRole != RoleParticipant {
		t.Fatalf("expected role %s, got %s", RoleParticipant, pRole)
	}
	if pRoom.ID != created.ID {
		t.Fatalf("expected room ID %s, got %s", created.ID, pRoom.ID)
	}

	// Lookup with invalid token
	_, _, err = store.GetByToken(ctx, "invalid-token-12345")
	if !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestStoreRoomExpiration(t *testing.T) {
	store, _ := setupTestStore(t)
	ctx := context.Background()

	// Create room with negative TTL to simulate immediate expiration
	created, err := store.Create(ctx, -time.Second, 10<<30, 2<<30, 100, "")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = store.GetByToken(ctx, created.ParticipantToken)
	if !errors.Is(err, ErrRoomExpired) {
		t.Fatalf("expected ErrRoomExpired, got %v", err)
	}

	_, _, err = store.GetByToken(ctx, created.CreatorToken)
	if !errors.Is(err, ErrRoomExpired) {
		t.Fatalf("expected ErrRoomExpired for creator as well, got %v", err)
	}
}

func TestStoreRoomEarlyClose(t *testing.T) {
	store, _ := setupTestStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, time.Hour, 10<<30, 2<<30, 100, "")
	if err != nil {
		t.Fatal(err)
	}

	// Participant cannot close room (using participant token as creator token fails)
	err = store.Close(ctx, created.ParticipantToken)
	if !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound when closing with participant token, got %v", err)
	}

	// Creator closes room
	err = store.Close(ctx, created.CreatorToken)
	if err != nil {
		t.Fatalf("creator close room failed: %v", err)
	}

	// After closing, lookup returns ErrRoomClosed
	_, _, err = store.GetByToken(ctx, created.ParticipantToken)
	if !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("expected ErrRoomClosed for participant, got %v", err)
	}

	_, _, err = store.GetByToken(ctx, created.CreatorToken)
	if !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("expected ErrRoomClosed for creator, got %v", err)
	}
}

func TestStoreRoomClosingLifecycle(t *testing.T) {
	store, _ := setupTestStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, time.Hour, 10<<30, 2<<30, 100, "")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial active state
	rm, role, err := store.GetByToken(ctx, created.ParticipantToken)
	if err != nil {
		t.Fatal(err)
	}
	if role != RoleParticipant || rm.Status != "active" || rm.IsClosing() {
		t.Fatalf("expected active room, got %s", rm.Status)
	}

	// 2. Start closing with 10-second duration
	closingRoom, err := store.StartClosing(ctx, created.ID, 10*time.Second)
	if err != nil {
		t.Fatalf("start closing failed: %v", err)
	}
	if closingRoom.Status != "closing" || !closingRoom.IsClosing() {
		t.Fatalf("expected status closing, got %s", closingRoom.Status)
	}
	if closingRoom.CloseDeadline == nil {
		t.Fatal("expected non-nil CloseDeadline")
	}
	remSec := closingRoom.ClosingRemainingSeconds()
	if remSec <= 0 || remSec > 10 {
		t.Fatalf("expected remaining seconds between 1 and 10, got %d", remSec)
	}

	// 3. Repeated StartClosing call does NOT extend or reset deadline
	initialDeadline := *closingRoom.CloseDeadline
	reClosingRoom, err := store.StartClosing(ctx, created.ID, 10*time.Second)
	if err != nil {
		t.Fatalf("repeat start closing failed: %v", err)
	}
	if reClosingRoom.Status != "closing" {
		t.Fatalf("expected status closing on repeat, got %s", reClosingRoom.Status)
	}
	if reClosingRoom.CloseDeadline.Unix() != initialDeadline.Unix() {
		t.Fatalf("expected deadline unix %v unchanged, got %v", initialDeadline.Unix(), reClosingRoom.CloseDeadline.Unix())
	}

	// 4. Querying during closing returns status "closing"
	closingQuery, _, err := store.GetByToken(ctx, created.ParticipantToken)
	if err != nil {
		t.Fatalf("expected no error during closing query, got %v", err)
	}
	if closingQuery.Status != "closing" {
		t.Fatalf("expected status closing from GetByToken, got %s", closingQuery.Status)
	}

	// 5. Update close_deadline to past to simulate deadline expiration
	pastDeadline := time.Now().UTC().Add(-2 * time.Second).Format(time.RFC3339)
	_, err = store.db.Exec("UPDATE rooms SET close_deadline = ? WHERE id = ?", pastDeadline, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 6. GetByToken detects past deadline and finalizes to closed
	_, _, err = store.GetByToken(ctx, created.ParticipantToken)
	if !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("expected ErrRoomClosed after deadline, got %v", err)
	}

	// 7. StartClosing on closed room returns ErrRoomClosed
	_, err = store.StartClosing(ctx, created.ID, 10*time.Second)
	if !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("expected ErrRoomClosed on StartClosing for already closed room, got %v", err)
	}
}

func TestValidatePIN(t *testing.T) {
	valid := []string{"1234", "0000", "98765432", "abcd", "A1b2", " 123456 "}
	for _, p := range valid {
		if err := ValidatePIN(p); err != nil {
			t.Errorf("expected PIN %q to be valid, got: %v", p, err)
		}
	}

	invalid := []string{"", "12", "123", "123456789", "12\x0034", "123\n"}
	for _, p := range invalid {
		if err := ValidatePIN(p); err == nil {
			t.Errorf("expected PIN %q to be invalid, got nil", p)
		}
	}
}

func TestHashAndVerifyPIN(t *testing.T) {
	secret := "secret-pepper-32-bytes-long-key"
	salt := "abcdef0123456789"
	pin := "4920"

	h1 := HashPIN(secret, salt, pin)
	h2 := HashPIN(secret, salt, pin)
	if h1 != h2 {
		t.Fatal("same inputs must produce identical PIN hash")
	}

	if !VerifyPIN(secret, salt, pin, h1) {
		t.Fatal("expected VerifyPIN to succeed with correct PIN")
	}
	if VerifyPIN(secret, salt, "wrong", h1) {
		t.Fatal("expected VerifyPIN to fail with incorrect PIN")
	}
	if VerifyPIN(secret, "other-salt", pin, h1) {
		t.Fatal("expected VerifyPIN to fail with different salt")
	}
	if VerifyPIN("other-secret", salt, pin, h1) {
		t.Fatal("expected VerifyPIN to fail with different secret")
	}
}

func TestStorePINAuthenticationAndLockout(t *testing.T) {
	store, _ := setupTestStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, time.Hour, 10<<30, 2<<30, 100, "8492")
	if err != nil {
		t.Fatal(err)
	}

	if !created.PinRequired {
		t.Fatal("expected room to require PIN")
	}

	// 1. Wrong PIN attempt 1
	ok, rem, retry, err := store.VerifyAndRecordPINAttempt(ctx, created.ParticipantToken, "0000")
	if ok || rem != 4 || retry != 0 || !errors.Is(err, ErrIncorrectPIN) {
		t.Fatalf("attempt 1: expected ok=false, rem=4, retry=0, got ok=%v, rem=%d, retry=%d, err=%v", ok, rem, retry, err)
	}

	// 2. Wrong PIN attempts 2, 3, 4
	for i := 2; i <= 4; i++ {
		ok, rem, retry, err = store.VerifyAndRecordPINAttempt(ctx, created.ParticipantToken, "0000")
		expectedRem := 5 - i
		if ok || rem != expectedRem || retry != 0 || !errors.Is(err, ErrIncorrectPIN) {
			t.Fatalf("attempt %d: expected rem=%d, got rem=%d, err=%v", i, expectedRem, rem, err)
		}
	}

	// 3. 5th wrong attempt triggers 5m cooldown
	ok, rem, retry, err = store.VerifyAndRecordPINAttempt(ctx, created.ParticipantToken, "0000")
	if ok || rem != 0 || retry <= 0 || !errors.Is(err, ErrRoomLocked) {
		t.Fatalf("attempt 5: expected ok=false, rem=0, retry>0, err=ErrRoomLocked, got ok=%v, rem=%d, retry=%d, err=%v", ok, rem, retry, err)
	}

	// Subsequent attempts while locked are blocked
	ok, _, retry, err = store.VerifyAndRecordPINAttempt(ctx, created.ParticipantToken, "8492")
	if ok || retry <= 0 || !errors.Is(err, ErrRoomLocked) {
		t.Fatalf("expected blocked attempt while locked, got ok=%v, retry=%d, err=%v", ok, retry, err)
	}

	// Creator is NOT affected and can unlock
	err = store.UnlockRoom(ctx, created.CreatorToken)
	if err != nil {
		t.Fatalf("creator unlock failed: %v", err)
	}

	// After unlock, correct PIN succeeds
	ok, rem, retry, err = store.VerifyAndRecordPINAttempt(ctx, created.ParticipantToken, "8492")
	if !ok || rem != 5 || retry != 0 || err != nil {
		t.Fatalf("expected success after unlock, got ok=%v, rem=%d, retry=%d, err=%v", ok, rem, retry, err)
	}
}

func TestStoreSessionLifecycle(t *testing.T) {
	store, _ := setupTestStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, time.Hour, 10<<30, 2<<30, 100, "1234")
	if err != nil {
		t.Fatal(err)
	}

	sessionToken, err := store.CreateSession(ctx, created.ID, created.ExpiresAt)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if !strings.HasPrefix(sessionToken, "s_") {
		t.Fatalf("expected prefix s_, got %q", sessionToken)
	}

	// Validate session
	valid, err := store.ValidateSession(ctx, created.ID, sessionToken)
	if err != nil || !valid {
		t.Fatalf("expected valid session, got valid=%v, err=%v", valid, err)
	}

	// Invalid session token
	valid, err = store.ValidateSession(ctx, created.ID, "s_invalidtoken1234567890")
	if err != nil || valid {
		t.Fatalf("expected invalid session to fail, got valid=%v, err=%v", valid, err)
	}

	// Cross-room session isolation
	created2, err := store.Create(ctx, time.Hour, 10<<30, 2<<30, 100, "1234")
	if err != nil {
		t.Fatal(err)
	}
	valid, err = store.ValidateSession(ctx, created2.ID, sessionToken)
	if err != nil || valid {
		t.Fatalf("expected session from room 1 to be invalid for room 2, got valid=%v, err=%v", valid, err)
	}
}

func TestGenerateSVG(t *testing.T) {
	svg, err := GenerateSVG("http://192.168.1.100:8080/r/p_test123", 280)
	if err != nil {
		t.Fatal(err)
	}
	if len(svg) == 0 {
		t.Fatal("expected non-empty SVG output")
	}
	if !bytes.HasPrefix(svg, []byte("<svg")) {
		t.Fatalf("expected SVG prefix, got %s", string(svg[:20]))
	}
	if !strings.Contains(string(svg), "viewBox=") {
		t.Fatal("expected viewBox attribute in SVG")
	}
	if !strings.Contains(string(svg), "</svg>") {
		t.Fatal("expected closing svg tag")
	}
}
