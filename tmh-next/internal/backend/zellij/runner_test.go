package zellij_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mark1708/tmh-next/internal/backend/zellij"
)

func TestExecRunnerKeepsDiagnosticsOutOfStructuredOutput(t *testing.T) {
	runner := zellij.ExecRunner{Binary: "sh"}
	output, err := runner.Run(context.Background(), "-c", "printf '[1]'; printf 'warning' >&2")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "[1]" {
		t.Fatalf("structured output = %q, want %q", output, "[1]")
	}
}

func TestExecRunnerRejectsOversizedOutput(t *testing.T) {
	runner := zellij.ExecRunner{Binary: "sh", OutputLimit: 4}
	_, err := runner.Run(context.Background(), "-c", "printf '12345'")
	if err == nil || !strings.Contains(err.Error(), "output exceeds 4 bytes") {
		t.Fatalf("oversize error = %v", err)
	}
}
