package text

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTextEmpty        = errors.New("text content cannot be empty")
	ErrTextTooLarge     = errors.New("text exceeds maximum permitted size")
	ErrTextLimitReached = errors.New("maximum text messages per room reached")
	ErrRoomInactive     = errors.New("room is inactive")
)

type Text struct {
	ID              string    `json:"id"`
	RoomID          string    `json:"room_id"`
	SenderType      string    `json:"sender_type"`                 // "client" or "server"
	SenderSessionID *string   `json:"sender_session_id,omitempty"` // nullable for server
	Content         string    `json:"content"`
	SizeBytes       int64     `json:"size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
	IsSelf          bool      `json:"is_self,omitempty"` // dynamically populated per request
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random text id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) CreateClientText(ctx context.Context, roomID, senderSessionID, content string, maxSizeBytes int64, maxTextsPerRoom int) (Text, error) {
	if strings.TrimSpace(content) == "" {
		return Text{}, ErrTextEmpty
	}

	sizeBytes := int64(len([]byte(content)))
	if maxSizeBytes > 0 && sizeBytes > maxSizeBytes {
		return Text{}, ErrTextTooLarge
	}

	textID, err := generateID()
	if err != nil {
		return Text{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Text{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Check room status
	var status string
	var expiresAtStr string
	err = tx.QueryRowContext(ctx, "SELECT status, expires_at FROM rooms WHERE id = ?", roomID).Scan(&status, &expiresAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Text{}, ErrRoomInactive
		}
		return Text{}, fmt.Errorf("query room status: %w", err)
	}

	if status != "active" {
		return Text{}, ErrRoomInactive
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		expiresAt, err = time.Parse("2006-01-02 15:04:05", expiresAtStr)
	}
	if err == nil && time.Now().UTC().After(expiresAt) {
		return Text{}, ErrRoomInactive
	}

	if maxTextsPerRoom > 0 {
		var currentCount int
		err = tx.QueryRowContext(ctx, "SELECT COUNT(id) FROM texts WHERE room_id = ?", roomID).Scan(&currentCount)
		if err != nil {
			return Text{}, fmt.Errorf("query text count: %w", err)
		}
		if currentCount >= maxTextsPerRoom {
			return Text{}, ErrTextLimitReached
		}
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	var sessionIDParam sql.NullString
	if strings.TrimSpace(senderSessionID) != "" {
		sessionIDParam = sql.NullString{String: strings.TrimSpace(senderSessionID), Valid: true}
	}

	query := `
		INSERT INTO texts (id, room_id, sender_type, sender_session_id, content, size_bytes, created_at)
		VALUES (?, ?, 'client', ?, ?, ?, ?);
	`
	_, err = tx.ExecContext(ctx, query, textID, roomID, sessionIDParam, content, sizeBytes, nowStr)
	if err != nil {
		return Text{}, fmt.Errorf("insert client text: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Text{}, fmt.Errorf("commit text creation: %w", err)
	}

	var sessPtr *string
	if sessionIDParam.Valid {
		sessPtr = &sessionIDParam.String
	}

	return Text{
		ID:              textID,
		RoomID:          roomID,
		SenderType:      "client",
		SenderSessionID: sessPtr,
		Content:         content,
		SizeBytes:       sizeBytes,
		CreatedAt:       now,
	}, nil
}

func (s *Store) CreateServerText(ctx context.Context, roomID, content string, maxTextsPerRoom int) (Text, error) {
	if strings.TrimSpace(content) == "" {
		return Text{}, ErrTextEmpty
	}

	sizeBytes := int64(len([]byte(content)))
	textID, err := generateID()
	if err != nil {
		return Text{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Text{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Check if room exists
	var status string
	err = tx.QueryRowContext(ctx, "SELECT status FROM rooms WHERE id = ?", roomID).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Text{}, ErrRoomInactive
		}
		return Text{}, fmt.Errorf("query room status: %w", err)
	}

	if maxTextsPerRoom > 0 {
		var currentCount int
		err = tx.QueryRowContext(ctx, "SELECT COUNT(id) FROM texts WHERE room_id = ?", roomID).Scan(&currentCount)
		if err != nil {
			return Text{}, fmt.Errorf("query text count: %w", err)
		}
		if currentCount >= maxTextsPerRoom {
			return Text{}, ErrTextLimitReached
		}
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	query := `
		INSERT INTO texts (id, room_id, sender_type, sender_session_id, content, size_bytes, created_at)
		VALUES (?, ?, 'server', NULL, ?, ?, ?);
	`
	_, err = tx.ExecContext(ctx, query, textID, roomID, content, sizeBytes, nowStr)
	if err != nil {
		return Text{}, fmt.Errorf("insert server text: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Text{}, fmt.Errorf("commit server text creation: %w", err)
	}

	return Text{
		ID:              textID,
		RoomID:          roomID,
		SenderType:      "server",
		SenderSessionID: nil,
		Content:         content,
		SizeBytes:       sizeBytes,
		CreatedAt:       now,
	}, nil
}

func (s *Store) ListRoomTexts(ctx context.Context, roomID string) ([]Text, error) {
	query := `
		SELECT id, room_id, sender_type, sender_session_id, content, size_bytes, created_at
		FROM texts
		WHERE room_id = ?
		ORDER BY created_at ASC, rowid ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("query texts: %w", err)
	}
	defer rows.Close()

	var results []Text
	for rows.Next() {
		var t Text
		var nullSession sql.NullString
		var createdAtStr string
		if err := rows.Scan(&t.ID, &t.RoomID, &t.SenderType, &nullSession, &t.Content, &t.SizeBytes, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scan text: %w", err)
		}
		if nullSession.Valid {
			sess := nullSession.String
			t.SenderSessionID = &sess
		}
		parsedTime, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			parsedTime, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		}
		t.CreatedAt = parsedTime
		results = append(results, t)
	}

	if results == nil {
		results = []Text{}
	}
	return results, nil
}

func (s *Store) CountRoomTexts(ctx context.Context, roomID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(id) FROM texts WHERE room_id = ?", roomID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count texts: %w", err)
	}
	return count, nil
}

