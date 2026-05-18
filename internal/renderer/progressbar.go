package renderer

// ProgressBar returns a 5-segment UTF-8 progress bar for the given percentage.
// Input is clamped to [0, 100]. Each segment represents 20% of the total.
// Within each segment: <10% of segment → empty ("░"), 10-<20% → half ("▒"),
// >=20% → full ("█"). The percentage is first rounded to the nearest 10.
//
// Canonical 11-point mapping (every 10%):
//
//	0%  → "░░░░░"
//	10% → "▒░░░░"
//	20% → "█░░░░"
//	...
//	100% → "█████"
func ProgressBar(percent float64) string {
	// Clamp to [0, 100].
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Truncate to the nearest lower multiple of 10 (floor).
	rounded := floorToNearest10(percent)

	// Each of the 5 segments covers 20 percentage points.
	// Segment value = share of 20pp that this segment represents:
	//   segVal = min(max(rounded - i*20, 0), 20)   for i in [0,4]
	const segWidth = 20.0
	const numSeg = 5

	bar := make([]rune, numSeg)
	remaining := rounded
	for i := 0; i < numSeg; i++ {
		seg := remaining
		if seg > segWidth {
			seg = segWidth
		}
		if seg <= 0 {
			bar[i] = '░' // empty
		} else if seg < segWidth {
			bar[i] = '▒' // half (partially filled)
		} else {
			bar[i] = '█' // full
		}
		remaining -= segWidth
	}
	return string(bar)
}

// floorToNearest10 truncates v down to the nearest multiple of 10.
// Examples: 49 → 40, 50 → 50, 51 → 50, 100 → 100.
func floorToNearest10(v float64) float64 {
	r := int(v/10.0) * 10
	if r < 0 {
		r = 0
	}
	if r > 100 {
		r = 100
	}
	return float64(r)
}

// ProgressBarColor returns the ANSI colour code for a progress-bar value at
// the given percentage, selected by threshold:
//
//	< 50% → green
//	50–69% → yellow
//	70–89% → orange
//	≥ 90% → red
//
// Returns an empty string when theme.AnsiEnabled is false.
func ProgressBarColor(percent float64, th Theme) string {
	if !th.AnsiEnabled {
		return ""
	}
	switch {
	case percent < 50:
		return th.Colors.Green
	case percent < 70:
		return th.Colors.Yellow
	case percent < 90:
		return th.Colors.Orange
	default:
		return th.Colors.Red
	}
}
