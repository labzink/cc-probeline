// Package config_test contains benchmark tests for config loading.
// Test T-LB1 per phase-6-plan-6.b.md §3.4.
// Gate G7: parse of a typical ~700-byte config must complete in <1ms median.
package config_test

import (
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// BenchmarkConfigLoad measures the time to parse a ~600-byte TOML config
// file (all sections populated with default-equivalent values).
// Gate G7: median must be <1ms (1_000_000 ns).
//
// Run with:
//
//	go test -bench=BenchmarkConfigLoad -benchtime=100x -run=^$ ./tests/config/
func BenchmarkConfigLoad(b *testing.B) {
	path := filepath.Join("..", "fixtures", "config", "default-template.toml")
	abs, err := filepath.Abs(path)
	if err != nil {
		b.Fatalf("BenchmarkConfigLoad: filepath.Abs: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg, errs := config.Load(abs)
		// Prevent the compiler from optimising the call away.
		if cfg == nil {
			b.Fatal("BenchmarkConfigLoad: Load returned nil cfg")
		}
		for _, e := range errs {
			if e.Severity == config.SeverityError {
				b.Fatalf("BenchmarkConfigLoad: unexpected SeverityError: %v", e)
			}
		}
	}
}
