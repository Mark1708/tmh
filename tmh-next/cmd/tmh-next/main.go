// Command tmh-next runs the terminal-native control panel.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mark1708/tmh-next/internal/app"
	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/mock"
	"github.com/mark1708/tmh-next/internal/remote"
	"github.com/mark1708/tmh-next/internal/ui/pages"
)

func main() {
	start := app.RunCLI(os.Args[1:])
	var client control.Client
	var watcher control.Watcher
	var ticker control.DemoTicker
	live := false
	if start.Demo {
		demo := mock.NewClient(start.Scenario)
		client, ticker, live = demo, demo, start.Live
	} else {
		socket := start.Socket
		if socket == "" {
			socket = defaultSocket()
		}
		runtimeClient, err := remote.New(socket)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tmh-next:", err)
			os.Exit(1)
		}
		if err := ensureDaemon(socket, runtimeClient); err != nil {
			fmt.Fprintln(os.Stderr, "tmh-next:", err)
			os.Exit(1)
		}
		client, watcher = runtimeClient, runtimeClient
	}
	root := app.NewRoot(app.Options{
		Client: client, Watcher: watcher, DemoTicker: ticker,
		Scheduler: app.ProductionScheduler{Delay: 0}, Live: live,
		Stack: start.Stack, QuickSwitch: start.QuickSwitch, Factory: pages.Factory,
	})
	if _, err := tea.NewProgram(root).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tmh-next:", err)
		os.Exit(1)
	}
}

func defaultSocket() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "tmh", "tmhd.sock")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "tmh", "tmhd.sock")
	}
	return filepath.Join(home, ".local", "state", "tmh", "run", "tmhd.sock")
}

func ensureDaemon(socket string, client *remote.Client) error {
	if daemonReady(client) {
		return nil
	}
	binary, err := findDaemon()
	if err != nil {
		return fmt.Errorf("connect %s: %w", socket, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(home, ".local", "state", "tmh")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("create daemon log directory: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "tmhd.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	command := exec.Command(binary, "--socket", socket)
	command.Stdin = nil
	command.Stdout, command.Stderr = logFile, logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start tmhd: %w", err)
	}
	_ = command.Process.Release()
	_ = logFile.Close()
	for range 50 {
		if daemonReady(client) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("tmhd did not become ready on %s", socket)
}

func daemonReady(client *remote.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	_, err := client.Snapshot(ctx)
	return err == nil
}

func findDaemon() (string, error) {
	if executable, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(executable), "tmhd")
		if info, statErr := os.Stat(sibling); statErr == nil && !info.IsDir() {
			return sibling, nil
		}
	}
	if binary, err := exec.LookPath("tmhd"); err == nil {
		return binary, nil
	}
	if cwd, err := os.Getwd(); err == nil {
		local := filepath.Join(cwd, "bin", "tmhd")
		if info, statErr := os.Stat(local); statErr == nil && !info.IsDir() {
			return local, nil
		}
	}
	return "", fmt.Errorf("tmhd binary not found beside tmh-next or on PATH")
}
