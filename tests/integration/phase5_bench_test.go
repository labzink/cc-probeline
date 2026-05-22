//go:build integration

package integration_test

// phase5_bench_test.go — cold-start CLI benchmark for Phase 5.
//
// Run:
//
//	go test -tags=integration -bench=BenchmarkColdStartCLI -benchtime=10x -run=^$ ./tests/integration/
//
// Target: median < 100 ms per invocation.
// Hard fail in CI: > 300 ms (reported as a log warning, not a test failure,
// per plan §2.2 — the benchmark itself is the metric, not an assertion).

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BenchmarkColdStartCLI measures the wall-clock time for a single cc-probeline
// invocation: process start → stdin decode → render → stdout → exit.
//
// The binary is built once via phase5BinPath (sync.Once shared with e2e tests).
// The fixture is read once before the timer starts. Each iteration spawns a
// fresh OS process to simulate the real CC status-line refresh pattern.
//
// Plan §2.2 / §7.4: target median < 100 ms, hard advisory > 300 ms.
func BenchmarkColdStartCLI(b *testing.B) {
	bin := phase5BinPath(b)

	// Read fixture once.
	root, err := findProjectRoot()
	if err != nil {
		b.Fatalf("BenchmarkColdStartCLI: findProjectRoot: %v", err)
	}
	fixturePath := filepath.Join(root, "tests/fixtures/integration/phase5/short-cli.json")
	stdin, err := os.ReadFile(fixturePath)
	if err != nil {
		b.Fatalf("BenchmarkColdStartCLI: ReadFile fixture: %v", err)
	}

	// Isolated HOME so the binary does not touch the real ~/.claude.
	home := b.TempDir()
	env := append(os.Environ(), "HOME="+home)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cmd := exec.Command(bin) //nolint:gosec
		cmd.Stdin = bytes.NewReader(stdin)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		cmd.Env = env
		if err := cmd.Run(); err != nil {
			b.Fatalf("BenchmarkColdStartCLI iter %d: %v", i, err)
		}
	}
	// ns/op reported by the framework is the SLA metric; target <100ms, hard-fail >300ms.
	// No inline assertion per plan §2.2 — the benchmark result itself is the signal.
}
