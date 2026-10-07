package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mark1708/tmh-next/internal/backend/zellij"
	"github.com/mark1708/tmh-next/internal/daemonapi"
	"github.com/mark1708/tmh-next/internal/production"
	"github.com/mark1708/tmh-next/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tmhd:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	runtimeHome := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeHome == "" {
		runtimeHome = filepath.Join(stateHome, "tmh", "run")
	}
	defaultSocket := filepath.Join(runtimeHome, "tmh", "tmhd.sock")
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		defaultSocket = filepath.Join(runtimeHome, "tmhd.sock")
	}

	var socketPath, statePath, configPath, zellijBinary string
	var pollInterval, operationTimeout time.Duration
	flag.StringVar(&socketPath, "socket", defaultSocket, "Unix socket path")
	flag.StringVar(&statePath, "state", filepath.Join(stateHome, "tmh", "state.db"), "SQLite state path")
	flag.StringVar(&configPath, "config", filepath.Join(home, ".config", "tmh", "config.yml"), "existing tmh YAML config")
	flag.StringVar(&zellijBinary, "zellij", "zellij", "Zellij binary")
	flag.DurationVar(&pollInterval, "poll", 2*time.Second, "runtime reconciliation interval")
	flag.DurationVar(&operationTimeout, "operation-timeout", 30*time.Second, "maximum snapshot or command duration")
	flag.Parse()
	if pollInterval < 250*time.Millisecond {
		return fmt.Errorf("poll interval must be at least 250ms")
	}
	if operationTimeout < time.Second {
		return fmt.Errorf("operation timeout must be at least 1s")
	}

	listener, err := listenUnix(socketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	database, err := store.Open(statePath)
	if err != nil {
		return err
	}
	defer database.Close()
	runner := zellij.ExecRunner{Binary: zellijBinary}
	adapter := zellij.New(runner, zellij.Options{ManagedPrefix: "tmh-", MinimumVersion: "0.45.1"})
	client, err := production.NewClient(production.Options{
		Discoverer: adapter, Runner: runner, Store: database, ConfigPath: configPath,
		CaptureHistory: true, ZellijBinary: zellijBinary, OperationTimeout: operationTimeout,
	})
	if err != nil {
		return err
	}
	api, err := daemonapi.New(client)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if _, err := client.Snapshot(ctx); err != nil {
		slog.Warn("initial runtime reconciliation failed", "error", err)
	}
	go reconcileLoop(ctx, client, pollInterval)

	server := &http.Server{
		Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	slog.Info("tmhd ready", "socket", socketPath, "state", statePath)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func reconcileLoop(ctx context.Context, client *production.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := client.Snapshot(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("runtime reconciliation failed", "error", err)
			}
		}
	}
}

func listenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("protect socket directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to replace non-socket path %s", path)
		}
		connection, dialErr := net.DialTimeout("unix", path, 300*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return nil, fmt.Errorf("daemon already listening on %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("protect socket: %w", err)
	}
	return listener, nil
}
