package parser

import "testing"

// T-S1: windowsSlug canonicalizes a Windows cwd to the slug Claude Code uses
// under %USERPROFILE%\.claude\projects\.
//
// BL-9 (partial): the expected slugs encode the best-effort formula (drive colon
// + both separators -> "-"). They are NOT yet verified against a real folder on
// a Windows install; this test locks the formula we ship so a future Windows
// hands-on can confirm or correct it. The function is GOOS-independent, so the
// case runs on any platform.
func TestWindowsSlug(t *testing.T) {
	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{"drive_root_path", `C:\Users\X\proj`, "C--Users-X-proj"},
		{"nested_path", `C:\Users\Konstantin\Projects\cc-probeline`, "C--Users-Konstantin-Projects-cc-probeline"},
		{"d_drive", `D:\work\repo`, "D--work-repo"},
		{"forward_slashes", `C:/Users/X/proj`, "C--Users-X-proj"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowsSlug(tc.cwd); got != tc.want {
				t.Errorf("windowsSlug(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}
}
