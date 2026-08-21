package room

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

type Role string

const (
	RoleCreator     Role = "creator"
	RoleParticipant Role = "participant"
)

// DeriveParticipantToken deterministically and securely derives the participant token
// from the creator token using a one-way HMAC-SHA256 derivation.
func DeriveParticipantToken(creatorToken string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(creatorToken)))
	mac.Write([]byte("lan-drop-participant-v1"))
	derived := mac.Sum(nil)[:24]
	return "p_" + base64.RawURLEncoding.EncodeToString(derived)
}

// GenerateTokens generates a cryptographically secure random creator token
// and its derived one-way participant token.
func GenerateTokens() (creatorToken, participantToken string, err error) {
	creatorBytes := make([]byte, 24)
	if _, err := rand.Read(creatorBytes); err != nil {
		return "", "", fmt.Errorf("generate creator token: %w", err)
	}

	creatorToken = "c_" + base64.RawURLEncoding.EncodeToString(creatorBytes)
	participantToken = DeriveParticipantToken(creatorToken)
	return creatorToken, participantToken, nil
}

// GenerateRoomID generates a random 16-byte hex identifier for internal room identification.
func GenerateRoomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate room id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// HashToken computes an HMAC-SHA256 of the token using the server's persisted secret.
func HashToken(serverSecret, token string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(serverSecret)))
	mac.Write([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(mac.Sum(nil))
}
