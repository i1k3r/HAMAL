package room

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRoomNotFound = errors.New("room not found")
	ErrRoomExpired  = errors.New("room has expired")
	ErrRoomClosed   = errors.New("room is closed")
	ErrUnauthorized = errors.New("unauthorized action")
)

type Room struct {
	ID                   string     `json:"id"`
	CreatorTokenHash     string     `json:"-"`
	ParticipantTokenHash string     `json:"-"`
	CreatedAt            time.Time  `json:"created_at"`
	ExpiresAt            time.Time  `json:"expires_at"`
	TTLSeconds           int        `json:"ttl_seconds"`
	Status               string     `json:"status"`
	PinRequired          bool       `json:"pin_required"`
	PinHash              string     `json:"-"`
	PinSalt              string     `json:"-"`
	PinAttempts          int        `json:"-"`
	LockedUntil          *time.Time `json:"-"`
	MaxRoomSize          int64      `json:"max_room_size"`
	MaxFileSize          int64      `json:"max_file_size"`
	MaxFiles             int        `json:"max_files"`
}

func (r *Room) IsLocked() bool {
	if r.LockedUntil == nil {
		return false
	}
	return time.Now().UTC().Before(*r.LockedUntil)
}

func (r *Room) LockoutRemainingSeconds() int {
	if !r.IsLocked() {
		return 0
	}
	rem := int(time.Until(*r.LockedUntil).Seconds())
	if rem < 0 {
		return 0
	}
	return rem
}

type CreatedRoom struct {
	Room
	CreatorToken     string `json:"creator_token"`
	ParticipantToken string `json:"participant_token"`
}

type Store struct {
	db           *sql.DB
	serverSecret string
}

func NewStore(db *sql.DB, serverSecret string) *Store {
	return &Store{
		db:           db,
		serverSecret: serverSecret,
	}
}

func (s *Store) Create(ctx context.Context, ttl time.Duration, maxRoomSize, maxFileSize int64, maxFiles int, pin string) (*CreatedRoom, error) {
	roomID, err := GenerateRoomID()
	if err != nil {
		return nil, err
	}

	creatorToken, participantToken, err := GenerateTokens()
	if err != nil {
		return nil, err
	}

	creatorHash := HashToken(s.serverSecret, creatorToken)
	participantHash := HashToken(s.serverSecret, participantToken)

	now := time.Now().UTC()
	expiresAt := now.Add(ttl).UTC()
	ttlSeconds := int(ttl.Seconds())

	pinRequiredInt := 0
	var pinHash, pinSalt string
	trimmedPIN := strings.TrimSpace(pin)
	if trimmedPIN != "" {
		if err := ValidatePIN(trimmedPIN); err != nil {
			return nil, err
		}
		pinSalt, err = GeneratePINSalt()
		if err != nil {
			return nil, err
		}
		pinHash = HashPIN(s.serverSecret, pinSalt, trimmedPIN)
		pinRequiredInt = 1
	}

	query := `
		INSERT INTO rooms (
			id, creator_token_hash, participant_token_hash,
			created_at, expires_at, ttl_seconds, status,
			pin_required, pin_hash, pin_salt, pin_attempts, locked_until,
			max_room_size, max_file_size, max_files
		) VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, 0, NULL, ?, ?, ?);
	`

	_, err = s.db.ExecContext(
		ctx,
		query,
		roomID,
		creatorHash,
		participantHash,
		now.Format(time.RFC3339),
		expiresAt.Format(time.RFC3339),
		ttlSeconds,
		pinRequiredInt,
		pinHash,
		pinSalt,
		maxRoomSize,
		maxFileSize,
		maxFiles,
	)
	if err != nil {
		return nil, fmt.Errorf("insert room: %w", err)
	}

	created := &CreatedRoom{
		Room: Room{
			ID:                   roomID,
			CreatorTokenHash:     creatorHash,
			ParticipantTokenHash: participantHash,
			CreatedAt:            now,
			ExpiresAt:            expiresAt,
			TTLSeconds:           ttlSeconds,
			Status:               "active",
			PinRequired:          (pinRequiredInt == 1),
			PinHash:              pinHash,
			PinSalt:              pinSalt,
			PinAttempts:          0,
			LockedUntil:          nil,
			MaxRoomSize:          maxRoomSize,
			MaxFileSize:          maxFileSize,
			MaxFiles:             maxFiles,
		},
		CreatorToken:     creatorToken,
		ParticipantToken: participantToken,
	}

	return created, nil
}

func (s *Store) GetByToken(ctx context.Context, rawToken string) (*Room, Role, error) {
	tokenHash := HashToken(s.serverSecret, rawToken)

	query := `
		SELECT id, creator_token_hash, participant_token_hash,
		       created_at, expires_at, ttl_seconds, status,
		       pin_required, pin_hash, pin_salt, pin_attempts, locked_until,
		       max_room_size, max_file_size, max_files
		FROM rooms
		WHERE creator_token_hash = ? OR participant_token_hash = ?;
	`

	var r Room
	var createdAtStr, expiresAtStr string
	var pinHashNull, pinSaltNull, lockedUntilNull sql.NullString
	var pinRequiredInt, pinAttempts int

	err := s.db.QueryRowContext(ctx, query, tokenHash, tokenHash).Scan(
		&r.ID,
		&r.CreatorTokenHash,
		&r.ParticipantTokenHash,
		&createdAtStr,
		&expiresAtStr,
		&r.TTLSeconds,
		&r.Status,
		&pinRequiredInt,
		&pinHashNull,
		&pinSaltNull,
		&pinAttempts,
		&lockedUntilNull,
		&r.MaxRoomSize,
		&r.MaxFileSize,
		&r.MaxFiles,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrRoomNotFound
		}
		return nil, "", fmt.Errorf("query room: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		r.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
		r.ExpiresAt = t
	}
	r.PinRequired = (pinRequiredInt == 1)
	r.PinHash = pinHashNull.String
	r.PinSalt = pinSaltNull.String
	r.PinAttempts = pinAttempts
	if lockedUntilNull.Valid && lockedUntilNull.String != "" {
		if t, err := time.Parse(time.RFC3339, lockedUntilNull.String); err == nil {
			r.LockedUntil = &t
		}
	}

	var role Role
	if tokenHash == r.CreatorTokenHash {
		role = RoleCreator
	} else {
		role = RoleParticipant
	}

	if r.Status == "closed" {
		return &r, role, ErrRoomClosed
	}

	if time.Now().UTC().After(r.ExpiresAt) {
		r.Status = "expired"
		return &r, role, ErrRoomExpired
	}

	return &r, role, nil
}

// VerifyAndRecordPINAttempt verifies the submitted PIN, handles atomic attempt incrementing,
// and enforces temporary lockout cooldowns when attempts exceed thresholds.
func (s *Store) VerifyAndRecordPINAttempt(ctx context.Context, participantToken, rawPIN string) (bool, int, int, error) {
	rm, role, err := s.GetByToken(ctx, participantToken)
	if err != nil {
		return false, 0, 0, err
	}
	if role != RoleParticipant {
		return false, 0, 0, ErrUnauthorized
	}
	if !rm.PinRequired {
		return true, 5, 0, nil
	}

	if rm.IsLocked() {
		return false, 0, rm.LockoutRemainingSeconds(), ErrRoomLocked
	}

	matched := VerifyPIN(s.serverSecret, rm.PinSalt, rawPIN, rm.PinHash)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var currentAttempts int
	var lockedUntilNull sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT pin_attempts, locked_until FROM rooms WHERE id = ?", rm.ID).Scan(&currentAttempts, &lockedUntilNull)
	if err != nil {
		return false, 0, 0, fmt.Errorf("query locked state: %w", err)
	}

	if lockedUntilNull.Valid && lockedUntilNull.String != "" {
		if t, err := time.Parse(time.RFC3339, lockedUntilNull.String); err == nil {
			if time.Now().UTC().Before(t) {
				rem := int(time.Until(t).Seconds())
				if rem < 0 {
					rem = 0
				}
				return false, 0, rem, ErrRoomLocked
			}
		}
	}

	if matched {
		_, _ = tx.ExecContext(ctx, `UPDATE rooms SET pin_attempts = 0, locked_until = NULL WHERE id = ?`, rm.ID)
		if err := tx.Commit(); err != nil {
			return false, 0, 0, err
		}
		return true, 5, 0, nil
	}

	newAttempts := currentAttempts + 1
	lockDuration := CalculateLockoutDuration(newAttempts)

	var lockedUntilStr *string
	var retryAfterSec int
	if lockDuration > 0 {
		t := time.Now().UTC().Add(lockDuration).Format(time.RFC3339)
		lockedUntilStr = &t
		retryAfterSec = int(lockDuration.Seconds())
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE rooms SET pin_attempts = ?, locked_until = ? WHERE id = ?`,
		newAttempts,
		lockedUntilStr,
		rm.ID,
	)
	if err != nil {
		return false, 0, 0, fmt.Errorf("record pin attempt: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, 0, 0, err
	}

	remaining := 5 - newAttempts
	if remaining < 0 {
		remaining = 0
	}

	if lockDuration > 0 {
		return false, 0, retryAfterSec, ErrRoomLocked
	}

	return false, remaining, 0, ErrIncorrectPIN
}

// UnlockRoom allows the creator to reset participant failed attempts and cooldown.
func (s *Store) UnlockRoom(ctx context.Context, creatorToken string) error {
	creatorHash := HashToken(s.serverSecret, creatorToken)

	res, err := s.db.ExecContext(
		ctx,
		`UPDATE rooms SET pin_attempts = 0, locked_until = NULL WHERE creator_token_hash = ? AND status = 'active'`,
		creatorHash,
	)
	if err != nil {
		return fmt.Errorf("unlock room: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRoomNotFound
	}
	return nil
}

// CreateSession creates a random session token, records its HMAC hash in SQLite, and returns the raw token.
func (s *Store) CreateSession(ctx context.Context, roomID string, expiresAt time.Time) (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	sessionToken := "s_" + base64.RawURLEncoding.EncodeToString(bytes)
	sessionTokenHash := HashToken(s.serverSecret, sessionToken)
	sessionID, err := GenerateRoomID()
	if err != nil {
		return "", err
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	expiresAtStr := expiresAt.Format(time.RFC3339)

	query := `
		INSERT INTO room_sessions (id, room_id, session_token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?);
	`
	_, err = s.db.ExecContext(ctx, query, sessionID, roomID, sessionTokenHash, nowStr, expiresAtStr)
	if err != nil {
		return "", fmt.Errorf("insert room session: %w", err)
	}

	return sessionToken, nil
}

// ValidateSession verifies if the given session token is valid and unexpired for the specified room.
func (s *Store) ValidateSession(ctx context.Context, roomID, rawSessionToken string) (bool, error) {
	token := strings.TrimSpace(rawSessionToken)
	if token == "" {
		return false, nil
	}
	sessionTokenHash := HashToken(s.serverSecret, token)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	query := `
		SELECT COUNT(*)
		FROM room_sessions
		WHERE room_id = ? AND session_token_hash = ? AND expires_at > ?;
	`
	var count int
	err := s.db.QueryRowContext(ctx, query, roomID, sessionTokenHash, nowStr).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("validate session: %w", err)
	}
	return count > 0, nil
}

func (s *Store) Close(ctx context.Context, creatorToken string) error {
	creatorHash := HashToken(s.serverSecret, creatorToken)

	query := `
		UPDATE rooms
		SET status = 'closed'
		WHERE creator_token_hash = ? AND status = 'active';
	`

	res, err := s.db.ExecContext(ctx, query, creatorHash)
	if err != nil {
		return fmt.Errorf("close room: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		var exists int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rooms WHERE creator_token_hash = ?", creatorHash).Scan(&exists)
		if err == nil && exists > 0 {
			return ErrRoomClosed
		}
		return ErrRoomNotFound
	}

	return nil
}
