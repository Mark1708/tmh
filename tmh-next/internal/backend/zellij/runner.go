package zellij

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
)

const defaultOutputLimit = 4 << 20

type ExecRunner struct {
	Binary      string
	OutputLimit int
}

func (r ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	binary := r.Binary
	if binary == "" {
		binary = "zellij"
	}
	limit := r.OutputLimit
	if limit <= 0 {
		limit = defaultOutputLimit
	}
	stdout := &cappedBuffer{remaining: limit}
	stderr := &cappedBuffer{remaining: limit}
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("%s %v: %w: %s", binary, args, err, stderr.String())
	}
	if stdout.Truncated() || stderr.Truncated() {
		return nil, fmt.Errorf("%s %v: output exceeds %d bytes", binary, args, limit)
	}
	return stdout.Bytes(), nil
}

type cappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	originalLength := len(p)
	if len(p) > b.remaining {
		p = p[:max(0, b.remaining)]
		b.truncated = true
	}
	if len(p) > 0 {
		_, _ = b.buf.Write(p)
		b.remaining -= len(p)
	}
	return originalLength, nil
}

func (b *cappedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *cappedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
