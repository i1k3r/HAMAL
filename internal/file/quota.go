package file

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrQuotaExceeded         = errors.New("room storage quota exceeded")
	ErrGlobalStorageExceeded = errors.New("global storage quota exceeded")
	ErrInsufficientStorage   = errors.New("insufficient filesystem free space")
)

type reservation struct {
	id     string
	roomID string
	bytes  int64
}

// QuotaManager provides thread-safe atomic in-flight storage quota reservations
// for both per-room limits and global storage limits to prevent race conditions during concurrent uploads.
type QuotaManager struct {
	mu            sync.Mutex
	reservations  map[string]reservation
	roomReserved  map[string]int64
	totalReserved int64
}

func NewQuotaManager() *QuotaManager {
	return &QuotaManager{
		reservations: make(map[string]reservation),
		roomReserved: make(map[string]int64),
	}
}

// Acquire attempts to reserve storage bytes for an in-flight upload in the specified room,
// validating both room capacity and global storage capacity atomically.
// Returns a reservation ID on success, or an error if capacity is insufficient.
func (qm *QuotaManager) Acquire(
	roomID string,
	requestedBytes int64,
	currentRoomUsage int64,
	maxRoomSize int64,
	currentGlobalUsage int64,
	maxTotalStorage int64,
) (string, error) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if requestedBytes <= 0 {
		return "", errors.New("requested bytes must be positive")
	}

	// 1. Check per-room quota
	activeRoomReserved := qm.roomReserved[roomID]
	if currentRoomUsage+activeRoomReserved+requestedBytes > maxRoomSize {
		return "", ErrQuotaExceeded
	}

	// 2. Check global quota (if maxTotalStorage > 0)
	if maxTotalStorage > 0 {
		if currentGlobalUsage+qm.totalReserved+requestedBytes > maxTotalStorage {
			return "", ErrGlobalStorageExceeded
		}
	}

	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate reservation id: %w", err)
	}
	resID := "res_" + hex.EncodeToString(bytes)

	qm.reservations[resID] = reservation{
		id:     resID,
		roomID: roomID,
		bytes:  requestedBytes,
	}
	qm.roomReserved[roomID] = activeRoomReserved + requestedBytes
	qm.totalReserved += requestedBytes

	return resID, nil
}

// Release frees an active reservation when an upload finishes or fails.
func (qm *QuotaManager) Release(reservationID string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	res, exists := qm.reservations[reservationID]
	if !exists {
		return
	}

	qm.roomReserved[res.roomID] -= res.bytes
	if qm.roomReserved[res.roomID] <= 0 {
		delete(qm.roomReserved, res.roomID)
	}
	qm.totalReserved -= res.bytes
	if qm.totalReserved <= 0 {
		qm.totalReserved = 0
	}
	delete(qm.reservations, reservationID)
}

// GetActiveReserved returns the total currently reserved in-flight bytes for a room.
func (qm *QuotaManager) GetActiveReserved(roomID string) int64 {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	return qm.roomReserved[roomID]
}

// GetTotalActiveReserved returns the total currently reserved in-flight bytes across all rooms.
func (qm *QuotaManager) GetTotalActiveReserved() int64 {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	return qm.totalReserved
}
