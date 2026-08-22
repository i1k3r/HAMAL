package app

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

func NewLogger(format, level string) *slog.Logger {
	var parsed slog.Level
	_ = parsed.UnmarshalText([]byte(level))
	options := &slog.HandlerOptions{Level: parsed}
	if format == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, options))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, options))
}
func requestLogging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request completed", "request_id", requestID, "method", r.Method, "path", safePath(r.URL.Path), "duration_ms", time.Since(started).Milliseconds())
	})
}
func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(bytes)
}
func safePath(path string) string {
	if strings.HasPrefix(path, "/r/") {
		return "/r/:room_token"
	}
	if strings.HasPrefix(path, "/c/") {
		return "/c/:creator_token"
	}
	if strings.HasPrefix(path, "/s/") {
		sub := strings.TrimPrefix(path, "/s/")
		if strings.HasSuffix(sub, "/download") {
			return "/s/:share_token/download"
		}
		return "/s/:share_token"
	}
	if strings.HasPrefix(path, "/api/v1/rooms/") {
		sub := strings.TrimPrefix(path, "/api/v1/rooms/")
		if strings.HasSuffix(sub, "/close") {
			return "/api/v1/rooms/:creator_token/close"
		}
		if strings.HasSuffix(sub, "/unlock") {
			return "/api/v1/rooms/:creator_token/unlock"
		}
		if strings.HasSuffix(sub, "/auth/pin") {
			return "/api/v1/rooms/:room_token/auth/pin"
		}
		if strings.HasSuffix(sub, "/qr.svg") {
			return "/api/v1/rooms/:room_token/qr.svg"
		}
		if strings.Contains(sub, "/files/") {
			if strings.HasSuffix(sub, "/share") {
				return "/api/v1/rooms/:creator_token/files/:file_id/share"
			}
			return "/api/v1/rooms/:room_token/files/:file_id"
		}
		if strings.Contains(sub, "/shares/") && strings.HasSuffix(sub, "/revoke") {
			return "/api/v1/rooms/:creator_token/shares/:share_id/revoke"
		}
		if strings.HasSuffix(sub, "/files") {
			return "/api/v1/rooms/:room_token/files"
		}
		return "/api/v1/rooms/:room_token"
	}
	return path
}
