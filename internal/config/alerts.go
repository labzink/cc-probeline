package config

import (
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ToCacheEvents synthesises a single parser.CacheEvent of type ConfigError when
// errs contains any SeverityError entries. Warnings alone do not trigger an alert.
// One event per render (collapse all errors — user runs check-config for details).
// Returns nil when input is empty or contains only warnings.
func ToCacheEvents(errs []Error) []parser.CacheEvent {
	for _, e := range errs {
		if e.Severity == SeverityError {
			return []parser.CacheEvent{{
				Type:      parser.ConfigError,
				Timestamp: time.Now(),
			}}
		}
	}
	return nil
}
