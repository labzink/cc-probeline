// Package stdin_test — T-14..T-16: black-box tests for RateLimits decoding.
// These tests are RED until internal/stdin.Payload gains the RateLimits field
// and "rate_limits" is added to knownTopLevelKeys.
package stdin_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestDecode_RateLimits_UnixTS (T-14) verifies that a payload with
// rate_limits containing Unix timestamp resets_at values is decoded into
// Payload.RateLimits without error in non-strict mode.
//
// RED: fails until Payload gains the RateLimits field.
func TestDecode_RateLimits_UnixTS(t *testing.T) {
	const raw = `{"rate_limits":{"five_hour":{"used_percentage":40,"resets_at":1745700000},"seven_day":{"used_percentage":60,"resets_at":1746305000}}}`

	p, err := stdin.Decode(strings.NewReader(raw), false)
	if err != nil {
		t.Fatalf("Decode RateLimits Unix TS: unexpected error: %v", err)
	}
	if p.RateLimits == nil {
		t.Fatal("Payload.RateLimits: want non-nil, got nil")
	}
	if p.RateLimits.FiveHour.UsedPercentage != 40.0 {
		t.Errorf("FiveHour.UsedPercentage: want 40.0, got %v", p.RateLimits.FiveHour.UsedPercentage)
	}
	if p.RateLimits.SevenDay.UsedPercentage != 60.0 {
		t.Errorf("SevenDay.UsedPercentage: want 60.0, got %v", p.RateLimits.SevenDay.UsedPercentage)
	}
	// ResetsAt must be non-empty raw bytes — actual time.Time parsing is
	// exercised via QuotaProbe render tests (T-18).
	if len(p.RateLimits.FiveHour.ResetsAt) == 0 {
		t.Error("FiveHour.ResetsAt: want non-empty raw bytes, got empty")
	}
}

// TestDecode_RateLimits_RFC3339 (T-15) verifies that a payload with
// rate_limits containing RFC3339 string resets_at values is decoded without
// error and that the raw bytes are preserved verbatim.
//
// RED: fails until Payload gains the RateLimits field.
func TestDecode_RateLimits_RFC3339(t *testing.T) {
	const raw = `{"rate_limits":{"five_hour":{"used_percentage":89,"resets_at":"2026-04-26T23:00:00Z"},"seven_day":{"used_percentage":45,"resets_at":"2026-05-02T18:00:00Z"}}}`

	p, err := stdin.Decode(strings.NewReader(raw), false)
	if err != nil {
		t.Fatalf("Decode RateLimits RFC3339: unexpected error: %v", err)
	}
	if p.RateLimits == nil {
		t.Fatal("Payload.RateLimits: want non-nil, got nil")
	}
	if p.RateLimits.FiveHour.UsedPercentage != 89.0 {
		t.Errorf("FiveHour.UsedPercentage: want 89.0, got %v", p.RateLimits.FiveHour.UsedPercentage)
	}
	if p.RateLimits.SevenDay.UsedPercentage != 45.0 {
		t.Errorf("SevenDay.UsedPercentage: want 45.0, got %v", p.RateLimits.SevenDay.UsedPercentage)
	}
	// The RFC3339 string is stored as-is in the raw JSON bytes (with quotes).
	rawStr := string(p.RateLimits.FiveHour.ResetsAt)
	if !strings.Contains(rawStr, "2026-04-26T23:00:00Z") {
		t.Errorf("FiveHour.ResetsAt: want raw bytes containing %q, got %q", "2026-04-26T23:00:00Z", rawStr)
	}
}

// TestDecode_RateLimits_KnownField (T-16) verifies that in strict mode
// "rate_limits" is treated as a known field and does NOT cause an error.
//
// RED: fails with "unknown field" until "rate_limits" is added to
// knownTopLevelKeys in internal/stdin/payload.go.
func TestDecode_RateLimits_KnownField(t *testing.T) {
	const raw = `{"rate_limits":{"five_hour":{"used_percentage":50,"resets_at":1745700000},"seven_day":{"used_percentage":50,"resets_at":1745700000}}}`

	_, err := stdin.Decode(strings.NewReader(raw), true)
	if err != nil {
		t.Fatalf("Decode RateLimits strict: want no error (rate_limits is known), got: %v", err)
	}
}

// Compile-time guard: ensure json.RawMessage is used in the test file so the
// import is recognised before the field exists.
var _ = json.RawMessage(nil)

// TestParseResetsAt_RFC3339 (T-15 direct) verifies that ParseResetsAt correctly
// parses a quoted RFC3339 string and returns ok=true with the expected time.Time.
func TestParseResetsAt_RFC3339(t *testing.T) {
	raw := json.RawMessage(`"2026-04-26T23:00:00Z"`)
	got, ok := stdin.ParseResetsAt(raw)
	if !ok {
		t.Fatal("ParseResetsAt(RFC3339): want ok=true, got false")
	}
	want := time.Date(2026, 4, 26, 23, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ParseResetsAt(RFC3339): want %v, got %v", want, got)
	}
}

// TestParseResetsAt_EmptySlice verifies that ParseResetsAt returns ok=false for
// an empty (nil / zero-length) raw message — no panic, no spurious time returned.
func TestParseResetsAt_EmptySlice(t *testing.T) {
	_, ok := stdin.ParseResetsAt(json.RawMessage(nil))
	if ok {
		t.Error("ParseResetsAt(nil): want ok=false, got true")
	}
	_, ok = stdin.ParseResetsAt(json.RawMessage{})
	if ok {
		t.Error("ParseResetsAt(empty slice): want ok=false, got true")
	}
}

// TestParseResetsAt_FloatAndNull verifies that ParseResetsAt returns ok=false for
// a JSON float value and for JSON null — neither form is a valid resets_at value
// (Insurance #2: third-form protection).
func TestParseResetsAt_FloatAndNull(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
	}{
		{"float", json.RawMessage(`1745700000.5`)},
		{"null", json.RawMessage(`null`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := stdin.ParseResetsAt(tc.raw)
			if ok {
				t.Errorf("ParseResetsAt(%s): want ok=false, got true", tc.name)
			}
		})
	}
}
