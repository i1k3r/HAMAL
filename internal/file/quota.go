package file

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrQuotaExceeded = errors.New("room storage quota exceeded")
)

type reservation struct {
	id     string
	roomID string
	bytes  int64
}

// QuotaManager provides thread-safe atomic in-flight storage quota reservations
// to prevent race conditions during concurrent uploads.
type QuotaManager struct {
	mu           sync.Mutex
	reservations map[string]reservation
	roomReserved map[string]int64
}

func NewQuotaManager() *QuotaManager {
	return &QuotaManager{
		reservations: make(map[string]reservation),
		roomReserved: make(map[string]int64),
	}
}

// Acquire attempts to reserve storage bytes for an in-flight upload in the specified room.
// Returns a reservation ID on success, or ErrQuotaExceeded if capacity is insufficient.
func (qm *QuotaManager) Acquire(roomID string, requestedBytes int64, currentUsage int64, maxRoomSize int64) (string, error) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if requestedBytes <= 0 {
		return "", errors.New("requested bytes must be positive")
	}

	activeReserved := qm.roomReserved[roomID]
	if currentUsage+activeReserved+requestedBytes > maxRoomSize {
		return "", ErrQuotaExceeded
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
	qm.roomReserved[roomID] = activeReserved + requestedBytes

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
	delete(qm.reservations, reservationID)
}

// GetActiveReserved returns the total currently reserved in-flight bytes for a room.
func (qm *QuotaManager) GetActiveReserved(roomID string) int64 {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	return qm.roomReserved[roomID]
}
