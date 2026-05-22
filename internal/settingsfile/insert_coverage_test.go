// Additional internal-package tests to reach ≥90% coverage for insert.go.
// External tests live in tests/settingsfile/insert_test.go; they do not count
// toward the -cover ./internal/settingsfile/ measurement.
package settingsfile

import (
	"errors"
	"testing"
)

// TestInsertInternal_EmptySettings covers the basic insert path.
func TestInsertInternal_EmptySettings(t *testing.T) {
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", RefreshInterval: 5}
	got, err := InsertStatusLine(Settings{}, opts)
	if err != nil {
		t.Fatalf("InsertStatusLine error = %v; want nil", err)
	}
	if _, ok := got["statusLine"]; !ok {
		t.Fatal("statusLine absent")
	}
}

// TestInsertInternal_DefaultRefreshInterval covers the ri==0 branch.
func TestInsertInternal_DefaultRefreshInterval(t *testing.T) {
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", RefreshInterval: 0}
	got, err := InsertStatusLine(Settings{}, opts)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	block, _ := got["statusLine"].(map[string]any)
	if block["refreshInterval"] != 5 {
		t.Fatalf("refreshInterval = %v; want 5", block["refreshInterval"])
	}
}

// TestInsertInternal_IdempotentOurBlock covers the deep-equal idempotency path.
func TestInsertInternal_IdempotentOurBlock(t *testing.T) {
	const binPath = "/usr/local/bin/cc-probeline"
	input := Settings{
		"statusLine": map[string]any{
			"type":            "command",
			"command":         binPath,
			"padding":         float64(0),
			"refreshInterval": float64(5),
		},
	}
	opts := InsertOpts{BinaryPath: binPath, RefreshInterval: 5}
	got, err := InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	block, _ := got["statusLine"].(map[string]any)
	if block["command"] != binPath {
		t.Fatalf("command = %q; want %q", block["command"], binPath)
	}
}

// TestInsertInternal_UpdatesOurBlock covers the replace-our-block path.
func TestInsertInternal_UpdatesOurBlock(t *testing.T) {
	input := Settings{
		"statusLine": map[string]any{
			"type":            "command",
			"command":         "/old/path/cc-probeline",
			"padding":         float64(0),
			"refreshInterval": float64(5),
		},
	}
	opts := InsertOpts{BinaryPath: "/new/path/cc-probeline", RefreshInterval: 5}
	got, err := InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	block, _ := got["statusLine"].(map[string]any)
	if block["command"] != "/new/path/cc-probeline" {
		t.Fatalf("command = %q; want /new/path/cc-probeline", block["command"])
	}
}

// TestInsertInternal_RefusesForeign covers the foreign-no-force path.
func TestInsertInternal_RefusesForeign(t *testing.T) {
	input := Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-tool",
		},
	}
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", Force: false}
	_, err := InsertStatusLine(input, opts)
	if !errors.Is(err, ErrForeignStatusLine) {
		t.Fatalf("error = %v; want ErrForeignStatusLine", err)
	}
}

// TestInsertInternal_ForceOverwrites covers the force-overwrite-foreign path.
func TestInsertInternal_ForceOverwrites(t *testing.T) {
	input := Settings{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "/usr/local/bin/other-tool",
		},
	}
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", Force: true}
	got, err := InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	block, _ := got["statusLine"].(map[string]any)
	if block["command"] != opts.BinaryPath {
		t.Fatalf("command = %q; want %q", block["command"], opts.BinaryPath)
	}
}

// TestInsertInternal_NonMapStatusLine covers the non-map statusLine branch (treated as foreign).
func TestInsertInternal_NonMapStatusLine(t *testing.T) {
	input := Settings{"statusLine": "not-a-map"}
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", Force: false}
	_, err := InsertStatusLine(input, opts)
	if !errors.Is(err, ErrForeignStatusLine) {
		t.Fatalf("error = %v; want ErrForeignStatusLine", err)
	}
}

// TestInsertInternal_NonMapStatusLineForce covers the non-map statusLine with Force=true.
func TestInsertInternal_NonMapStatusLineForce(t *testing.T) {
	input := Settings{"statusLine": "not-a-map"}
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", Force: true}
	got, err := InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("error = %v; want nil with Force=true", err)
	}
	if _, ok := got["statusLine"]; !ok {
		t.Fatal("statusLine absent after Force overwrite")
	}
}

// TestInsertInternal_PreservesOtherKeys covers that non-statusLine keys survive.
func TestInsertInternal_PreservesOtherKeys(t *testing.T) {
	input := Settings{"theme": "dark"}
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", RefreshInterval: 5}
	got, err := InsertStatusLine(input, opts)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if got["theme"] != "dark" {
		t.Fatalf("theme = %v; want dark", got["theme"])
	}
}

// TestInsertInternal_InputNotMutated verifies the immutable contract.
func TestInsertInternal_InputNotMutated(t *testing.T) {
	input := Settings{"theme": "dark"}
	opts := InsertOpts{BinaryPath: "/usr/local/bin/cc-probeline", RefreshInterval: 5}
	_, _ = InsertStatusLine(input, opts)
	if _, ok := input["statusLine"]; ok {
		t.Fatal("InsertStatusLine mutated input map")
	}
}
