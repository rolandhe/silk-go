package dsp

import (
	"math"
	"testing"
)

// TestSigmQ15Bounds — clamps at 0/32767 outside ±192 (= 6*32 in Q5).
func TestSigmQ15Bounds(t *testing.T) {
	if got := SigmQ15(192); got != 32767 {
		t.Errorf("SigmQ15(192)=%d want 32767", got)
	}
	if got := SigmQ15(1000); got != 32767 {
		t.Errorf("SigmQ15(1000)=%d want 32767", got)
	}
	if got := SigmQ15(-192); got != 0 {
		t.Errorf("SigmQ15(-192)=%d want 0", got)
	}
	if got := SigmQ15(-1000); got != 0 {
		t.Errorf("SigmQ15(-1000)=%d want 0", got)
	}
}

// TestSigmQ15Center — sigm(0) ≈ 0.5 → 16384 in Q15.
func TestSigmQ15Center(t *testing.T) {
	if got := SigmQ15(0); got != 16384 {
		t.Errorf("SigmQ15(0)=%d want 16384", got)
	}
}

// TestSigmQ15AccuracyVsFloat — 6-point piecewise-linear LUT in [-6, 6].
// Max error ~1.5% in the steep region; anything tighter than 2% would be
// checking the LUT's design tradeoff, not correctness.
func TestSigmQ15AccuracyVsFloat(t *testing.T) {
	for q := int32(-180); q <= 180; q++ {
		x := float64(q) / 32.0
		want := 1.0 / (1.0 + math.Exp(-x))
		got := float64(SigmQ15(q)) / 32767.0
		if math.Abs(got-want) > 0.02 {
			t.Errorf("SigmQ15(%d): got %.4f want %.4f", q, got, want)
		}
	}
}

// TestSigmQ15Symmetry — sigm(-x) = 1 - sigm(x), within rounding tolerance.
func TestSigmQ15Symmetry(t *testing.T) {
	for q := int32(0); q <= 180; q++ {
		pos := SigmQ15(q)
		neg := SigmQ15(-q)
		// pos + neg should be ~32768 (= 1.0 in Q15) with small rounding.
		sum := pos + neg
		if sum < 32750 || sum > 32780 {
			t.Errorf("SigmQ15(%d)+SigmQ15(%d)=%d, want ~32768", q, -q, sum)
		}
	}
}
