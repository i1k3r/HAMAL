package file

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/i1k3r/lan-drop/internal/storage"
)

var (
	ErrFileTooLarge     = errors.New("file exceeds maximum allowed size")
	ErrFileLimitReached = errors.New("room file count limit reached")
	ErrEmptyFile        = errors.New("file is empty")
	ErrInvalidFilename  = errors.New("invalid filename")
	ErrFileNotFound     = errors.New("file not found")
)

type File struct {
	ID               string    `json:"file_id"`
	RoomID           string    `json:"-"`
	StorageID        string    `json:"-"`
	OriginalFilename string    `json:"filename"`
	SizeBytes        int64     `json:"size_bytes"`
	ContentType      string    `json:"content_type"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
}

type Store struct {
	db    *sql.DB
	paths storage.Paths
	quota *QuotaManager
}

func NewStore(db *sql.DB, paths storage.Paths, quota *QuotaManager) *Store {
	return &Store{
		db:    db,
		paths: paths,
		quota: quota,
	}
}

// SanitizeFilename cleans the input filename to remove directory separators,
// control characters, leading/trailing whitespace and dots, capping length to 255 chars.
func SanitizeFilename(raw string) string {
	clean := filepath.Base(strings.TrimSpace(raw))
	clean = strings.ReplaceAll(clean, "\\", "_")
	clean = strings.ReplaceAll(clean, "/", "_")

	var b strings.Builder
	for _, r := range clean {
		if unicode.IsControl(r) || r == 0 {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	res := strings.TrimSpace(b.String())
	res = strings.Trim(res, ".")

	if res == "" || res == "." || res == ".." {
		return "unnamed_file"
	}

	runes := []rune(res)
	if len(runes) > 255 {
		ext := filepath.Ext(res)
		extRunes := []rune(ext)
		if len(extRunes) < 50 && len(extRunes) > 0 {
			maxBase := 255 - len(extRunes)
			res = string(runes[:maxBase]) + ext
		} else {
			res = string(runes[:255])
		}
	}
	return res
}

// SanitizeContentDisposition formats a Content-Disposition header conforming to RFC 6266 / RFC 5987,
// safely escaping CRLF, quotes, and backslashes, while providing an RFC 5987 UTF-8 encoding parameter.
func SanitizeContentDisposition(filename string) string {
	var asciiBuilder strings.Builder
	for _, r := range filename {
		if r >= 0x20 && r <= 0x7e && r != '"' && r != '\\' && r != ';' && r != '\r' && r != '\n' {
			asciiBuilder.WriteRune(r)
		} else {
			asciiBuilder.WriteRune('_')
		}
	}
	asciiName := strings.TrimSpace(asciiBuilder.String())
	asciiName = strings.Trim(asciiName, ".")
	if asciiName == "" || asciiName == "." || asciiName == ".." {
		asciiName = "download"
	}

	// RFC 5987 percent-encoded UTF-8 filename
	utf8Encoded := url.PathEscape(filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, asciiName, utf8Encoded)
}

func GenerateFileID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate file id: %w", err)
	}
	return "f_" + hex.EncodeToString(bytes), nil
}

func GenerateStorageID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate storage id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GetRoomUsageAndCount returns the total finalized ready bytes and ready file count for a room.
func (s *Store) GetRoomUsageAndCount(ctx context.Context, roomID string) (int64, int, error) {
	query := `
		SELECT COALESCE(SUM(size_bytes), 0), COUNT(*)
		FROM files
		WHERE room_id = ? AND status = 'ready';
	`
	var usedBytes int64
	var count int
	err := s.db.QueryRowContext(ctx, query, roomID).Scan(&usedBytes, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("query room usage: %w", err)
	}
	return usedBytes, count, nil
}

// ListReadyFiles returns all files in a room with status = 'ready'.
func (s *Store) ListReadyFiles(ctx context.Context, roomID string) ([]File, error) {
	query := `
		SELECT id, room_id, storage_id, original_filename, size_bytes, content_type, status, created_at, completed_at
		FROM files
		WHERE room_id = ? AND status = 'ready'
		ORDER BY created_at ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	defer rows.Close()

	var list []File
	for rows.Next() {
		var f File
		var createdStr, completedStr string
		if err := rows.Scan(
			&f.ID, &f.RoomID, &f.StorageID, &f.OriginalFilename,
			&f.SizeBytes, &f.ContentType, &f.Status,
			&createdStr, &completedStr,
		); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			f.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, completedStr); err == nil {
			f.CompletedAt = t
		}
		list = append(list, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// GetReadyFile retrieves metadata for a specific ready file in a room.
func (s *Store) GetReadyFile(ctx context.Context, roomID, fileID string) (*File, error) {
	query := `
		SELECT id, room_id, storage_id, original_filename, size_bytes, content_type, status, created_at, completed_at
		FROM files
		WHERE id = ? AND room_id = ? AND status = 'ready';
	`
	var f File
	var createdStr, completedStr string
	err := s.db.QueryRowContext(ctx, query, fileID, roomID).Scan(
		&f.ID, &f.RoomID, &f.StorageID, &f.OriginalFilename,
		&f.SizeBytes, &f.ContentType, &f.Status,
		&createdStr, &completedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("query ready file: %w", err)
	}
	if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
		f.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, completedStr); err == nil {
		f.CompletedAt = t
	}
	return &f, nil
}

// OpenStorageFile securely opens the physical file on disk from /data/files by storage ID.
func (s *Store) OpenStorageFile(storageID string) (*os.File, error) {
	for _, r := range storageID {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return nil, ErrInvalidFilename
		}
	}
	cleanPath := filepath.Clean(filepath.Join(s.paths.FilesDir, storageID))
	rel, err := filepath.Rel(s.paths.FilesDir, cleanPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrInvalidFilename
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}
	return file, nil
}

// StreamUpload streams an incoming file directly into /data/staging, validates limits,
// atomically finalizes to /data/files, and records the file metadata in SQLite.
func (s *Store) StreamUpload(
	ctx context.Context,
	roomID string,
	rawFilename string,
	declaredContentType string,
	r io.Reader,
	declaredSize int64,
	maxFileSize int64,
	maxRoomSize int64,
	maxFiles int,
) (*File, error) {
	currentUsage, count, err := s.GetRoomUsageAndCount(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if count >= maxFiles {
		return nil, ErrFileLimitReached
	}

	remainingRoomQuota := maxRoomSize - currentUsage
	if remainingRoomQuota <= 0 {
		return nil, ErrQuotaExceeded
	}

	reservationSize := declaredSize
	if reservationSize <= 0 || reservationSize > maxFileSize {
		reservationSize = maxFileSize
	}
	if reservationSize > remainingRoomQuota {
		reservationSize = remainingRoomQuota
	}

	resID, err := s.quota.Acquire(roomID, reservationSize, currentUsage, maxRoomSize)
	if err != nil {
		return nil, err
	}
	defer s.quota.Release(resID)

	fileID, err := GenerateFileID()
	if err != nil {
		return nil, err
	}

	storageID, err := GenerateStorageID()
	if err != nil {
		return nil, err
	}

	filename := SanitizeFilename(rawFilename)
	stagingPath := filepath.Join(s.paths.StagingDir, "upload_"+storageID+".tmp")
	finalPath := filepath.Join(s.paths.FilesDir, storageID)

	out, err := os.OpenFile(stagingPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create staging file: %w", err)
	}

	stagingCleaned := false
	defer func() {
		if !stagingCleaned {
			_ = out.Close()
			_ = os.Remove(stagingPath)
		}
	}()

	// Maximum allowable bytes for this specific stream
	remainingQuota := maxRoomSize - currentUsage
	allowedBytes := maxFileSize
	if remainingQuota < allowedBytes {
		allowedBytes = remainingQuota
	}

	// Stream with buffer and limit check
	buf := make([]byte, 32*1024)
	var written int64
	var headerBuf []byte
	headerCollected := false

	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			if written+int64(n) > allowedBytes {
				return nil, ErrFileTooLarge
			}

			if !headerCollected {
				headerBuf = append(headerBuf, buf[:n]...)
				if len(headerBuf) >= 512 {
					headerCollected = true
				}
			}

			wn, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return nil, fmt.Errorf("write staging file: %w", writeErr)
			}
			written += int64(wn)
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read upload stream: %w", readErr)
		}
	}

	if err := out.Sync(); err != nil {
		return nil, fmt.Errorf("sync staging file: %w", err)
	}
	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("close staging file: %w", err)
	}
	stagingCleaned = true

	if written == 0 {
		_ = os.Remove(stagingPath)
		return nil, ErrEmptyFile
	}

	contentType := strings.TrimSpace(declaredContentType)
	if contentType == "" || contentType == "application/octet-stream" {
		if len(headerBuf) > 0 {
			contentType = http.DetectContentType(headerBuf)
		} else {
			contentType = "application/octet-stream"
		}
	}

	// Atomically move from staging to finalized storage
	if err := os.Rename(stagingPath, finalPath); err != nil {
		_ = os.Remove(stagingPath)
		return nil, fmt.Errorf("finalize storage file: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	insertQuery := `
		INSERT INTO files (id, room_id, storage_id, original_filename, size_bytes, content_type, status, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, 'ready', ?, ?);
	`
	_, err = s.db.ExecContext(ctx, insertQuery, fileID, roomID, storageID, filename, written, contentType, now, now)
	if err != nil {
		_ = os.Remove(finalPath)
		return nil, fmt.Errorf("record file in database: %w", err)
	}

	return &File{
		ID:               fileID,
		RoomID:           roomID,
		StorageID:        storageID,
		OriginalFilename: filename,
		SizeBytes:        written,
		ContentType:      contentType,
		Status:           "ready",
		CreatedAt:        time.Now().UTC(),
		CompletedAt:      time.Now().UTC(),
	}, nil
}
