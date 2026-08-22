package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeCreatesRequiredDirectories(t *testing.T) {
	paths, err := Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.DataDir, paths.FilesDir, paths.StagingDir, paths.SecretsDir} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s: %v", path, err)
		}
	}
	if err := Check(paths); err != nil {
		t.Fatal(err)
	}
}

func TestResolveServerSecretPersistsGeneratedValue(t *testing.T) {
	paths, err := Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := ResolveServerSecret(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveServerSecret(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("expected persistent generated secret")
	}
	if _, err := os.Stat(filepath.Join(paths.SecretsDir, "server-secret")); err != nil {
		t.Fatal(err)
	}
}

func TestResolveServerSecretRejectsMalformedPersistedSecret(t *testing.T) {
	paths, err := Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(paths.SecretsDir, "server-secret")
	shortSecret := []byte("too-short")
	if err := os.WriteFile(secretPath, shortSecret, 0600); err != nil {
		t.Fatal(err)
	}

	_, err = ResolveServerSecret(paths, "")
	if err == nil {
		t.Fatal("expected error for malformed persisted secret, got nil")
	}

	// Verify the file was NOT overwritten with a new secret
	content, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(shortSecret) {
		t.Fatalf("expected malformed secret not to be overwritten, got %q", string(content))
	}
}

func TestResolveServerSecretRejectsShortProvidedSecret(t *testing.T) {
	paths, err := Initialize(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ResolveServerSecret(paths, "short")
	if err == nil {
		t.Fatal("expected error for short provided secret, got nil")
	}
}

func TestCheckFreeSpace(t *testing.T) {
	tempDir := t.TempDir()
	freeBytes, err := CheckFreeSpace(tempDir)
	if err != nil {
		t.Fatalf("CheckFreeSpace failed: %v", err)
	}
	if freeBytes == 0 {
		t.Fatal("expected non-zero free disk space")
	}
}
