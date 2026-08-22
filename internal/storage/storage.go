package storage

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	DataDir, FilesDir, StagingDir, SecretsDir string
}

func Initialize(dataDir string) (Paths, error) {
	paths := Paths{
		DataDir:    dataDir,
		FilesDir:   filepath.Join(dataDir, "files"),
		StagingDir: filepath.Join(dataDir, "staging"),
		SecretsDir: filepath.Join(dataDir, "secrets"),
	}
	for _, path := range []string{paths.DataDir, paths.FilesDir, paths.StagingDir, paths.SecretsDir} {
		if err := os.MkdirAll(path, 0750); err != nil {
			return Paths{}, fmt.Errorf("create %s: %w", path, err)
		}
		if err := checkWritable(path); err != nil {
			return Paths{}, err
		}
	}
	return paths, nil
}

func ResolveServerSecret(paths Paths, provided string) (string, error) {
	if provided != "" {
		if len(provided) < 32 {
			return "", fmt.Errorf("server secret must be at least 32 bytes")
		}
		return provided, nil
	}
	path := filepath.Join(paths.SecretsDir, "server-secret")
	content, err := os.ReadFile(path)
	if err == nil {
		secret := strings.TrimSpace(string(content))
		if len(secret) < 32 {
			return "", fmt.Errorf("persisted server secret at %s is malformed (must be at least 32 bytes)", path)
		}
		return secret, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read server secret: %w", err)
	}

	secret := make([]byte, 48)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate server secret: %w", err)
	}
	encoded := fmt.Sprintf("%x", secret)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		return "", fmt.Errorf("persist generated server secret: %w", err)
	}
	return encoded, nil
}

func Check(paths Paths) error {
	for _, path := range []string{paths.DataDir, paths.FilesDir, paths.StagingDir} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("storage path unavailable %s: %w", path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("storage path is not a directory: %s", path)
		}
		if err := checkWritable(path); err != nil {
			return err
		}
	}
	return nil
}
func checkWritable(path string) error {
	file, err := os.CreateTemp(path, ".lan-drop-check-")
	if err != nil {
		return fmt.Errorf("storage path is not writable %s: %w", path, err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Remove(name); err != nil {
		return err
	}
	return nil
}

// CheckFreeSpace queries the filesystem containing path for free bytes available to the caller.
func CheckFreeSpace(path string) (uint64, error) {
	return checkFreeSpaceOS(path)
}
