package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/i1k3r/lan-drop/internal/app"
	"github.com/i1k3r/lan-drop/internal/config"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := app.NewLogger(cfg.LogFormat, cfg.LogLevel)
	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer application.Close()
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := application.Ready(); err != nil {
			logger.Error("health check failed", "error", err)
			os.Exit(1)
		}
		return
	}

	// 1. Initial cleanup pass at application startup
	startupCleanupCtx, startupCleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if stats, err := application.RunCleanupOnce(startupCleanupCtx); err != nil {
		logger.Warn("initial startup cleanup encountered error", "error", err)
	} else if stats.RoomsCleaned > 0 || stats.FilesDeleted > 0 || stats.StagingCleaned > 0 || stats.OrphansCleaned > 0 {
		logger.Info("initial startup cleanup reclaimed resources",
			"rooms_cleaned", stats.RoomsCleaned,
			"files_deleted", stats.FilesDeleted,
			"staging_cleaned", stats.StagingCleaned,
			"orphans_cleaned", stats.OrphansCleaned,
		)
	}
	startupCleanupCancel()

	// 2. Start background cleanup worker
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	var workerWg sync.WaitGroup
	workerWg.Add(1)
	go func() {
		defer workerWg.Done()
		application.StartCleanup(workerCtx)
	}()

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "listen_addr", cfg.ListenAddr)
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-stop:
		logger.Info("shutdown signal received", "signal", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			workerCancel()
			workerWg.Wait()
			os.Exit(1)
		}
		workerCancel()
		workerWg.Wait()
		return
	}

	// Graceful worker shutdown
	workerCancel()
	workerWg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			logger.Error("forced shutdown failed", "error", closeErr)
		}
	}
	logger.Info("server stopped")
}
