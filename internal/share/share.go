package share

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/i1k3r/lan-drop/internal/file"
)

var (
	ErrShareNotFound     = errors.New("share not found")
	ErrShareExpired      = errors.New("share has expired")
	ErrShareRevoked      = errors.New("share is revoked")
	ErrRoomInactive      = errors.New("room is inactive")
	ErrShareLimitReached = errors.New("room share limit reached")
	ErrInvalidToken      = errors.New("invalid share token")
)

type Share struct {
	ID        string     `json:"share_id"`
	RoomID    string     `json:"-"`
	FileID    string     `json:"file_id"`
	TokenHash string     `json:"-"`
	Status    string     `json:"status"` // 'active', 'revoked'
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func (s *Share) IsActive() bool {
	return s.Status == "active" && time.Now().UTC().Before(s.ExpiresAt)
}

func (s *Share) RemainingSeconds() int {
	rem := int(time.Until(s.ExpiresAt).Seconds())
	if rem < 0 {
		return 0
	}
	return rem
}

func GenerateShareToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	return "gsh_" + hex.EncodeToString(bytes), nil
}

func HashShareToken(secret, token string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

type Store struct {
	db     *sql.DB
	secret string
}

func NewStore(db *sql.DB, secret string) *Store {
	return &Store{
		db:     db,
		secret: secret,
	}
}

func (s *Store) CreateShare(
	ctx context.Context,
	roomID string,
	fileID string,
	requestedTTL time.Duration,
	roomExpiresAt time.Time,
	maxShareTTL time.Duration,
	maxShares int,
) (*Share, string, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// Check active share count for this room
	var activeCount int
	countQuery := `
		SELECT COUNT(*)
		FROM shares
		WHERE room_id = ? AND status = 'active' AND expires_at > ?;
	`
	if err := s.db.QueryRowContext(ctx, countQuery, roomID, nowStr).Scan(&activeCount); err != nil {
		return nil, "", fmt.Errorf("check active share count: %w", err)
	}
	if activeCount >= maxShares {
		return nil, "", ErrShareLimitReached
	}

	// Verify file exists, belongs to room, and is in ready status
	var fileStatus string
	fileCheckQuery := `SELECT status FROM files WHERE id = ? AND room_id = ?;`
	if err := s.db.QueryRowContext(ctx, fileCheckQuery, fileID, roomID).Scan(&fileStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", file.ErrFileNotFound
		}
		return nil, "", fmt.Errorf("check file: %w", err)
	}
	if fileStatus != "ready" {
		return nil, "", file.ErrFileNotFound
	}

	// Calculate bounded share lifetime
	effectiveTTL := requestedTTL
	if effectiveTTL <= 0 || effectiveTTL > maxShareTTL {
		effectiveTTL = maxShareTTL
	}

	roomRemaining := roomExpiresAt.Sub(now)
	if roomRemaining <= 0 {
		return nil, "", ErrRoomInactive
	}
	if effectiveTTL > roomRemaining {
		effectiveTTL = roomRemaining
	}

	expiresAt := now.Add(effectiveTTL)
	expiresAtStr := expiresAt.Format(time.RFC3339)

	token, err := GenerateShareToken()
	if err != nil {
		return nil, "", err
	}
	tokenHash := HashShareToken(s.secret, token)

	shareIDBytes := make([]byte, 16)
	if _, err := rand.Read(shareIDBytes); err != nil {
		return nil, "", fmt.Errorf("generate share id: %w", err)
	}
	shareID := "sh_" + hex.EncodeToString(shareIDBytes)

	insertQuery := `
		INSERT INTO shares (
			id, room_id, file_id, token_hash, status, created_at, expires_at
		) VALUES (?, ?, ?, ?, 'active', ?, ?);
	`
	_, err = s.db.ExecContext(ctx, insertQuery, shareID, roomID, fileID, tokenHash, nowStr, expiresAtStr)
	if err != nil {
		return nil, "", fmt.Errorf("insert share: %w", err)
	}

	sh := &Share{
		ID:        shareID,
		RoomID:    roomID,
		FileID:    fileID,
		TokenHash: tokenHash,
		Status:    "active",
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	return sh, token, nil
}

func (s *Store) GetByToken(ctx context.Context, token string) (*Share, *file.File, error) {
	if !strings.HasPrefix(token, "gsh_") || len(token) != 68 {
		return nil, nil, ErrInvalidToken
	}
	for _, r := range token[4:] {
		if !unicode.Is(unicode.Hex_Digit, r) {
			return nil, nil, ErrInvalidToken
		}
	}

	tokenHash := HashShareToken(s.secret, token)

	query := `
		SELECT
			s.id, s.room_id, s.file_id, s.token_hash, s.status, s.created_at, s.expires_at, s.revoked_at,
			f.id, f.room_id, f.storage_id, f.original_filename, f.size_bytes, f.content_type, f.status, f.created_at, f.completed_at,
			r.status, r.expires_at
		FROM shares s
		JOIN files f ON s.file_id = f.id
		JOIN rooms r ON s.room_id = r.id
		WHERE s.token_hash = ?;
	`

	var sh Share
	var f file.File
	var shareCreatedStr, shareExpiresStr string
	var shareRevokedStr sql.NullString
	var fileCreatedStr, fileCompletedStr string
	var roomStatus, roomExpiresStr string

	err := s.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&sh.ID, &sh.RoomID, &sh.FileID, &sh.TokenHash, &sh.Status, &shareCreatedStr, &shareExpiresStr, &shareRevokedStr,
		&f.ID, &f.RoomID, &f.StorageID, &f.OriginalFilename, &f.SizeBytes, &f.ContentType, &f.Status, &fileCreatedStr, &fileCompletedStr,
		&roomStatus, &roomExpiresStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrShareNotFound
		}
		return nil, nil, fmt.Errorf("query share: %w", err)
	}

	// Parse timestamps
	if t, err := time.Parse(time.RFC3339, shareCreatedStr); err == nil {
		sh.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, shareExpiresStr); err == nil {
		sh.ExpiresAt = t
	}
	if shareRevokedStr.Valid {
		if t, err := time.Parse(time.RFC3339, shareRevokedStr.String); err == nil {
			sh.RevokedAt = &t
		}
	}
	if t, err := time.Parse(time.RFC3339, fileCreatedStr); err == nil {
		f.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, fileCompletedStr); err == nil {
		f.CompletedAt = t
	}

	var roomExpiresAt time.Time
	if t, err := time.Parse(time.RFC3339, roomExpiresStr); err == nil {
		roomExpiresAt = t
	}

	now := time.Now().UTC()

	// 1. Room-level isolation & status checks
	if roomStatus != "active" || (!roomExpiresAt.IsZero() && now.After(roomExpiresAt)) {
		return nil, nil, ErrRoomInactive
	}

	// 2. Share-level status checks
	if sh.Status == "revoked" {
		return nil, nil, ErrShareRevoked
	}
	if now.After(sh.ExpiresAt) {
		return nil, nil, ErrShareExpired
	}

	// 3. File status check
	if f.Status != "ready" {
		return nil, nil, ErrShareNotFound
	}

	return &sh, &f, nil
}

func (s *Store) RevokeShare(ctx context.Context, roomID, shareID string) error {
	nowStr := time.Now().UTC().Format(time.RFC3339)
	query := `
		UPDATE shares
		SET status = 'revoked', revoked_at = ?
		WHERE id = ? AND room_id = ? AND status = 'active';
	`
	res, err := s.db.ExecContext(ctx, query, nowStr, shareID, roomID)
	if err != nil {
		return fmt.Errorf("revoke share: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrShareNotFound
	}
	return nil
}

func (s *Store) ListRoomShares(ctx context.Context, roomID string) ([]Share, error) {
	query := `
		SELECT id, room_id, file_id, token_hash, status, created_at, expires_at, revoked_at
		FROM shares
		WHERE room_id = ?
		ORDER BY created_at DESC;
	`
	rows, err := s.db.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	defer rows.Close()

	var list []Share
	for rows.Next() {
		var sh Share
		var createdStr, expiresStr string
		var revokedStr sql.NullString
		if err := rows.Scan(
			&sh.ID, &sh.RoomID, &sh.FileID, &sh.TokenHash, &sh.Status,
			&createdStr, &expiresStr, &revokedStr,
		); err != nil {
			return nil, fmt.Errorf("scan share: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			sh.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, expiresStr); err == nil {
			sh.ExpiresAt = t
		}
		if revokedStr.Valid {
			if t, err := time.Parse(time.RFC3339, revokedStr.String); err == nil {
				sh.RevokedAt = &t
			}
		}
		list = append(list, sh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}
