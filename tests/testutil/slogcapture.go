// Package testutil provides shared test helpers for cc-probeline test suites.
package testutil

import (
	"context"
	"log/slog"
	"strings"
)

// CaptureHandler is a minimal slog.Handler that records log records so that
// tests can assert on warning messages without I/O side-effects.
type CaptureHandler struct {
	Records []slog.Record
}

// NewCaptureHandler returns a new CaptureHandler ready to use with slog.New.
func NewCaptureHandler() *CaptureHandler {
	return &CaptureHandler{}
}

func (h *CaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *CaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.Records = append(h.Records, r)
	return nil
}
func (h *CaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *CaptureHandler) WithGroup(_ string) slog.Handler      { return h }

// HasWarnContaining reports whether any captured record at Warn level
// has a message containing substr.
func (h *CaptureHandler) HasWarnContaining(substr string) bool {
	for _, r := range h.Records {
		if r.Level == slog.LevelWarn && strings.Contains(r.Message, substr) {
			return true
		}
	}
	return false
}
