package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrappingPreservesIdentity(t *testing.T) {
	tests := []struct {
		name   string
		target error
	}{
		{"server not running", ErrServerNotRunning},
		{"session exists", ErrSessionExists},
		{"schema violation", ErrSchemaViolation},
		{"unknown root", ErrUnknownRoot},
		{"template chain", ErrTemplateChain},
		{"hook denied", ErrHookDenied},
		{"state corrupted", ErrStateCorrupted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := fmt.Errorf("context: %w", tt.target)
			if !errors.Is(wrapped, tt.target) {
				t.Fatalf("errors.Is failed for %v", tt.target)
			}
		})
	}
}
