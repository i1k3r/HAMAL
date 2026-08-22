package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/i1k3r/HAMAL/internal/app"
	"github.com/i1k3r/HAMAL/internal/config"
)

// Options holds configuration for the HAMAL server facade.
type Options struct {
	DataDir      string
	ListenAddr   string
	BaseURL      string
	ServerSecret string
	LogLevel     string
	LogFormat    string
}

// Server encapsulates the in-process HAMAL backend application.
type Server struct {
	app    *app.App
	logger *slog.Logger
	cfg    config.Config
}

// New creates and initializes a new HAMAL Server instance using sensible defaults merged with provided options.
func New(opts Options, logger *slog.Logger) (*Server, error) {
	cfg := config.Default()

	if strings.TrimSpace(opts.DataDir) != "" {
		cfg.DataDir = opts.DataDir
		cfg.DBPath = filepath.Join(opts.DataDir, "lan-drop.db")
	}
	if strings.TrimSpace(opts.ListenAddr) != "" {
		cfg.ListenAddr = opts.ListenAddr
	}
	if strings.TrimSpace(opts.BaseURL) != "" {
		cfg.BaseURL = opts.BaseURL
	}
	if strings.TrimSpace(opts.ServerSecret) != "" {
		cfg.ServerSecret = opts.ServerSecret
	}
	if strings.TrimSpace(opts.LogLevel) != "" {
		cfg.LogLevel = opts.LogLevel
	}
	if strings.TrimSpace(opts.LogFormat) != "" {
		cfg.LogFormat = opts.LogFormat
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	if logger == nil {
		logger = app.NewLogger(cfg.LogFormat, cfg.LogLevel)
	}

	application, err := app.New(cfg, logger)
	if err != nil {
		return nil, err
	}

	return &Server{
		app:    application,
		logger: logger,
		cfg:    cfg,
	}, nil
}

// Handler returns the HTTP handler for the HAMAL server routes and embedded assets.
func (s *Server) Handler() http.Handler {
	return s.app.Handler()
}

// Ready verifies that the underlying database and storage are accessible.
func (s *Server) Ready() error {
	return s.app.Ready()
}

// StartCleanup runs the background cleanup loop until ctx is cancelled.
func (s *Server) StartCleanup(ctx context.Context) {
	s.app.StartCleanup(ctx)
}

// RunCleanupOnce performs a single cleanup pass on expired rooms, orphan files, and staging data.
func (s *Server) RunCleanupOnce(ctx context.Context) error {
	_, err := s.app.RunCleanupOnce(ctx)
	return err
}

// Close gracefully closes the database connection and frees server resources.
func (s *Server) Close() error {
	return s.app.Close()
}
