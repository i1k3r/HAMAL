package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr               string
	BaseURL                  string
	DataDir                  string
	DBPath                   string
	ServerSecret             string
	MaxTTL                   time.Duration
	DefaultTTL               time.Duration
	MinTTL                   time.Duration
	MaxFileSize              int64
	MaxRoomSize              int64
	MaxFilesPerRoom          int
	MaxTotalStorage          int64
	MinFreeSpace             int64
	CleanupInterval          time.Duration
	CleanupBatchSize         int
	StagingMaxAge            time.Duration
	OrphanGracePeriod        time.Duration
	ClosedRoomRetention      time.Duration
	GlobalShareEnabled       bool
	PublicBaseURL            string
	MaxShareTTL              time.Duration
	DefaultShareTTL          time.Duration
	MaxSharesPerRoom         int
	ShareManagementRateLimit int
	ShareAccessRateLimit     int
	UploadIdleTimeout        time.Duration
	LogFormat                string
	LogLevel                 string
	TrustedProxies           []string
	SecureCookies            string
}

func Default() Config {
	dataDir := "/data"
	return Config{
		ListenAddr: ":7700", DataDir: dataDir, DBPath: filepath.Join(dataDir, "lan-drop.db"),
		MaxTTL: 24 * time.Hour, DefaultTTL: time.Hour, MinTTL: 5 * time.Minute,
		MaxFileSize: 10 << 30, MaxRoomSize: 10 << 30, MaxFilesPerRoom: 100,
		MaxTotalStorage: 100 << 30, MinFreeSpace: 5 << 30,
		CleanupInterval: time.Minute, CleanupBatchSize: 50,
		StagingMaxAge: 15 * time.Minute, OrphanGracePeriod: 10 * time.Minute, ClosedRoomRetention: 0,
		GlobalShareEnabled: false, PublicBaseURL: "",
		MaxShareTTL: 24 * time.Hour, DefaultShareTTL: time.Hour,
		MaxSharesPerRoom: 10, ShareManagementRateLimit: 30, ShareAccessRateLimit: 300,
		UploadIdleTimeout: 10 * time.Minute,
		LogFormat:         "json", LogLevel: "info", SecureCookies: "auto",
	}
}

func LoadFromEnv() (Config, error) {
	c := Default()
	c.ListenAddr = envString("LAN_DROP_LISTEN_ADDR", c.ListenAddr)
	c.BaseURL = envString("LAN_DROP_BASE_URL", c.BaseURL)
	c.DataDir = envString("LAN_DROP_DATA_DIR", c.DataDir)
	c.DBPath = envString("LAN_DROP_DB_PATH", filepath.Join(c.DataDir, "lan-drop.db"))
	c.ServerSecret = strings.TrimSpace(os.Getenv("LAN_DROP_SERVER_SECRET"))
	c.LogFormat = strings.ToLower(envString("LAN_DROP_LOG_FORMAT", c.LogFormat))
	c.LogLevel = strings.ToLower(envString("LAN_DROP_LOG_LEVEL", c.LogLevel))
	c.SecureCookies = strings.ToLower(envString("LAN_DROP_SECURE_COOKIES", c.SecureCookies))
	c.TrustedProxies = splitCSV(os.Getenv("LAN_DROP_TRUSTED_PROXIES"))

	var err error
	if c.MaxTTL, err = envDuration("LAN_DROP_MAX_TTL", c.MaxTTL); err != nil {
		return c, err
	}
	if c.DefaultTTL, err = envDuration("LAN_DROP_DEFAULT_TTL", c.DefaultTTL); err != nil {
		return c, err
	}
	if c.MinTTL, err = envDuration("LAN_DROP_MIN_TTL", c.MinTTL); err != nil {
		return c, err
	}
	if c.CleanupInterval, err = envDuration("LAN_DROP_CLEANUP_INTERVAL", c.CleanupInterval); err != nil {
		return c, err
	}
	if c.CleanupBatchSize, err = envInt("LAN_DROP_CLEANUP_BATCH_SIZE", c.CleanupBatchSize); err != nil {
		return c, err
	}
	if c.StagingMaxAge, err = envDuration("LAN_DROP_STAGING_MAX_AGE", c.StagingMaxAge); err != nil {
		return c, err
	}
	if c.OrphanGracePeriod, err = envDuration("LAN_DROP_ORPHAN_GRACE_PERIOD", c.OrphanGracePeriod); err != nil {
		return c, err
	}
	if c.ClosedRoomRetention, err = envDuration("LAN_DROP_CLOSED_ROOM_RETENTION", c.ClosedRoomRetention); err != nil {
		return c, err
	}
	if c.GlobalShareEnabled, err = envBool("LAN_DROP_GLOBAL_SHARE_ENABLED", c.GlobalShareEnabled); err != nil {
		return c, err
	}
	c.PublicBaseURL = strings.TrimSpace(envString("LAN_DROP_PUBLIC_BASE_URL", c.PublicBaseURL))
	if c.MaxShareTTL, err = envDuration("LAN_DROP_MAX_SHARE_TTL", c.MaxShareTTL); err != nil {
		return c, err
	}
	if c.DefaultShareTTL, err = envDuration("LAN_DROP_DEFAULT_SHARE_TTL", c.DefaultShareTTL); err != nil {
		return c, err
	}
	if c.MaxSharesPerRoom, err = envInt("LAN_DROP_MAX_SHARES_PER_ROOM", c.MaxSharesPerRoom); err != nil {
		return c, err
	}
	if c.ShareManagementRateLimit, err = envInt("LAN_DROP_SHARE_MANAGEMENT_RATE_LIMIT", c.ShareManagementRateLimit); err != nil {
		return c, err
	}
	if c.ShareAccessRateLimit, err = envInt("LAN_DROP_SHARE_ACCESS_RATE_LIMIT", c.ShareAccessRateLimit); err != nil {
		return c, err
	}
	if c.UploadIdleTimeout, err = envDuration("LAN_DROP_UPLOAD_IDLE_TIMEOUT", c.UploadIdleTimeout); err != nil {
		return c, err
	}
	if c.MaxFileSize, err = envBytes("LAN_DROP_MAX_FILE_SIZE", c.MaxFileSize); err != nil {
		return c, err
	}
	if c.MaxRoomSize, err = envBytes("LAN_DROP_MAX_ROOM_SIZE", c.MaxRoomSize); err != nil {
		return c, err
	}
	if c.MaxTotalStorage, err = envBytes("LAN_DROP_MAX_TOTAL_STORAGE", c.MaxTotalStorage); err != nil {
		return c, err
	}
	if c.MinFreeSpace, err = envBytes("LAN_DROP_MIN_FREE_SPACE", c.MinFreeSpace); err != nil {
		return c, err
	}
	if c.MaxFilesPerRoom, err = envInt("LAN_DROP_MAX_FILES_PER_ROOM", c.MaxFilesPerRoom); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func ValidatePublicBaseURL(urlStr string) error {
	trimmed := strings.TrimSpace(urlStr)
	if trimmed == "" {
		return fmt.Errorf("public base URL cannot be empty")
	}
	u, err := url.ParseRequestURI(trimmed)
	if err != nil {
		return fmt.Errorf("invalid public base URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("LAN_DROP_PUBLIC_BASE_URL must use https scheme (got %q)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("LAN_DROP_PUBLIC_BASE_URL must specify a hostname")
	}
	if u.User != nil {
		return fmt.Errorf("LAN_DROP_PUBLIC_BASE_URL must not contain user credentials")
	}
	if strings.Contains(u.Path, "..") {
		return fmt.Errorf("LAN_DROP_PUBLIC_BASE_URL path must not contain relative traversal")
	}
	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("LAN_DROP_LISTEN_ADDR cannot be empty")
	}
	if strings.TrimSpace(c.DataDir) == "" || strings.TrimSpace(c.DBPath) == "" {
		return fmt.Errorf("data directory and database path are required")
	}
	absData, err := filepath.Abs(c.DataDir)
	if err != nil {
		return fmt.Errorf("invalid data directory: %w", err)
	}
	absDB, err := filepath.Abs(c.DBPath)
	if err != nil {
		return fmt.Errorf("invalid database path: %w", err)
	}
	rel, err := filepath.Rel(absData, absDB)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("database path %q must be located within data directory %q", c.DBPath, c.DataDir)
	}
	if c.BaseURL != "" {
		u, err := url.ParseRequestURI(c.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("LAN_DROP_BASE_URL must be an absolute URL")
		}
	}
	if c.PublicBaseURL != "" {
		if err := ValidatePublicBaseURL(c.PublicBaseURL); err != nil {
			return err
		}
	}
	if len(c.ServerSecret) > 0 && len(c.ServerSecret) < 32 {
		return fmt.Errorf("LAN_DROP_SERVER_SECRET must be at least 32 bytes")
	}
	if c.MinTTL <= 0 || c.DefaultTTL < c.MinTTL || c.MaxTTL < c.DefaultTTL {
		return fmt.Errorf("TTL values must satisfy 0 < minimum <= default <= maximum")
	}
	if c.MaxShareTTL <= 0 || c.DefaultShareTTL <= 0 || c.MaxShareTTL < c.DefaultShareTTL {
		return fmt.Errorf("share TTL values must satisfy 0 < default <= maximum")
	}
	if c.MaxFileSize <= 0 || c.MaxRoomSize <= 0 || c.MaxTotalStorage <= 0 || c.MinFreeSpace < 0 || c.MaxFilesPerRoom <= 0 {
		return fmt.Errorf("storage limits must be positive (minimum free space may be zero)")
	}
	if c.CleanupInterval <= 0 || c.CleanupBatchSize <= 0 || c.StagingMaxAge <= 0 || c.OrphanGracePeriod <= 0 || c.ClosedRoomRetention < 0 || c.UploadIdleTimeout <= 0 {
		return fmt.Errorf("cleanup parameters and upload idle timeout must be positive (closed room retention may be zero)")
	}
	if c.MaxSharesPerRoom <= 0 || c.ShareManagementRateLimit <= 0 || c.ShareAccessRateLimit <= 0 {
		return fmt.Errorf("share limits and rate limits must be positive")
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		return fmt.Errorf("LAN_DROP_LOG_FORMAT must be json or text")
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return fmt.Errorf("LAN_DROP_LOG_LEVEL must be debug, info, warn, or error")
	}
	if c.SecureCookies != "auto" && c.SecureCookies != "true" && c.SecureCookies != "false" {
		return fmt.Errorf("LAN_DROP_SECURE_COOKIES must be auto, true, or false")
	}
	for _, proxy := range c.TrustedProxies {
		if _, _, err := net.ParseCIDR(proxy); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q", proxy)
		}
	}
	return nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func envInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func envBytes(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	multiplier := int64(1)
	lower := strings.ToLower(value)
	units := []struct {
		suffix string
		factor int64
	}{
		{"gib", 1 << 30}, {"mib", 1 << 20}, {"kib", 1 << 10},
		{"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10}, {"b", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(lower, unit.suffix) {
			multiplier = unit.factor
			lower = strings.TrimSpace(strings.TrimSuffix(lower, unit.suffix))
			break
		}
	}
	parsed, err := strconv.ParseInt(lower, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative byte value", key)
	}
	return parsed * multiplier, nil
}

func splitCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func envBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", key, err)
	}
	return parsed, nil
}
