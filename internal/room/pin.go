package room

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

var (
	ErrInvalidPIN   = errors.New("PIN must be between 4 and 8 characters")
	ErrRoomLocked   = errors.New("room is temporarily locked due to too many failed PIN attempts")
	ErrIncorrectPIN = errors.New("incorrect PIN")
	ErrPINRequired  = errors.New("PIN authentication required")
)

// ValidatePIN validates that the PIN is between 4 and 8 characters long and contains valid characters.
func ValidatePIN(raw string) error {
	pin := strings.TrimSpace(raw)
	if len(pin) < 4 || len(pin) > 8 {
		return ErrInvalidPIN
	}
	for _, r := range pin {
		if unicode.IsControl(r) || r == 0 || r > 126 {
			return ErrInvalidPIN
		}
	}
	return nil
}

// GeneratePINSalt generates 16 cryptographically random bytes encoded as hex.
func GeneratePINSalt() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate pin salt: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// pbkdf2HMACSHA256 implements RFC 8018 PBKDF2 using HMAC-SHA256.
func pbkdf2HMACSHA256(password, salt []byte, iter, keyLen int) []byte {
	h := hmac.New(sha256.New, password)
	numBlocks := (keyLen + sha256.Size - 1) / sha256.Size
	buf := make([]byte, 4)
	dk := make([]byte, 0, numBlocks*sha256.Size)
	u := make([]byte, sha256.Size)
	t := make([]byte, sha256.Size)

	for block := 1; block <= numBlocks; block++ {
		h.Reset()
		h.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		h.Write(buf)
		u = h.Sum(u[:0])
		copy(t, u)

		for i := 2; i <= iter; i++ {
			h.Reset()
			h.Write(u)
			u = h.Sum(u[:0])
			for j := 0; j < sha256.Size; j++ {
				t[j] ^= u[j]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

// HashPIN derives a 256-bit key from the PIN using PBKDF2-HMAC-SHA256 with 100,000 iterations,
// incorporating the per-room salt and server-secret pepper.
func HashPIN(serverSecret, saltHex, pin string) string {
	saltBytes := append([]byte(strings.TrimSpace(saltHex)), []byte(strings.TrimSpace(serverSecret))...)
	derived := pbkdf2HMACSHA256([]byte(strings.TrimSpace(pin)), saltBytes, 100000, 32)
	return hex.EncodeToString(derived)
}

// VerifyPIN performs constant-time comparison of a submitted PIN against its expected hash.
func VerifyPIN(serverSecret, saltHex, pin, expectedHash string) bool {
	computed := HashPIN(serverSecret, saltHex, pin)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(expectedHash)) == 1
}

// CalculateLockoutDuration returns the tiered cooldown duration based on failed attempts count.
func CalculateLockoutDuration(attempts int) time.Duration {
	switch {
	case attempts < 5:
		return 0
	case attempts == 5:
		return 5 * time.Minute
	case attempts == 6:
		return 15 * time.Minute
	case attempts == 7:
		return 30 * time.Minute
	default:
		return 60 * time.Minute
	}
}
