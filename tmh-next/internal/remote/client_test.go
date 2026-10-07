package remote

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRejectsEmptySocketPath(t *testing.T) {
	client, err := New("")
	if err == nil || client != nil {
		t.Fatalf("New empty socket = (%v, %v), want error", client, err)
	}
}

func TestSnapshotReportsUnixSocketConnectionFailure(t *testing.T) {
	client, err := New(filepath.Join(t.TempDir(), "missing.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = client.Snapshot(ctx)
	if err == nil || !strings.Contains(err.Error(), "request tmhd") {
		t.Fatalf("Snapshot error = %v, want transport context", err)
	}
}
