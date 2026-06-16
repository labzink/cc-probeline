package hint

import (
	"fmt"
	"strconv"
	"strings"
)

// UpdateText returns the "update available" hint (#7c, Phase 7.46 Wave B / BL-36)
// when latest is a strictly higher semantic version than current; otherwise "".
//
// Both inputs are parsed leniently: a leading "v" and any pre-release/build
// suffix (after "-" or "+") are stripped, then the dot-separated numeric
// components are compared. A version that does not parse as numbers — notably the
// "dev" placeholder of an un-stamped build, or a malformed latest_version from a
// bad price file — yields "" so a development build never nags and a broken feed
// cannot invent an update. The returned string carries renderer color tokens.
func UpdateText(current, latest string) string {
	cv, okc := parseVersion(current)
	lv, okl := parseVersion(latest)
	if !okc || !okl || compareVersion(cv, lv) >= 0 {
		return ""
	}
	return fmt.Sprintf("{{color:green}}↑ update available: v%s → v%s{{reset}}",
		cleanVersion(current), cleanVersion(latest))
}

// cleanVersion trims surrounding space, a leading "v", and any pre-release/build
// suffix, leaving the bare "X.Y.Z" core.
func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

// parseVersion parses up to three dot-separated non-negative integers into a
// fixed [major, minor, patch] vector. Returns ok=false on any non-numeric or
// empty component, or more than three components.
func parseVersion(v string) ([3]int, bool) {
	c := cleanVersion(v)
	if c == "" {
		return [3]int{}, false
	}
	parts := strings.Split(c, ".")
	if len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// compareVersion returns -1, 0, or 1 as a is less than, equal to, or greater
// than b, comparing major then minor then patch.
func compareVersion(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}
