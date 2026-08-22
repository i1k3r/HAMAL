package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServerFacade(t *testing.T) {
	dataDir := t.TempDir()

	srv, err := New(Options{
		DataDir:    dataDir,
		ListenAddr: ":7700",
		LogLevel:   "debug",
		LogFormat:  "text",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.Close()

	if err := srv.Ready(); err != nil {
		t.Fatalf("server ready check failed: %v", err)
	}

	handler := srv.Handler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	// Test GET /healthz
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from /healthz, got %d", rec.Code)
	}

	// Test GET /readyz
	reqReady := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recReady := httptest.NewRecorder()
	handler.ServeHTTP(recReady, reqReady)

	if recReady.Code != http.StatusOK {
		t.Fatalf("expected status 200 from /readyz, got %d", recReady.Code)
	}

	// Test RunCleanupOnce
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.RunCleanupOnce(ctx); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}
