package dsp

import "testing"

// TestResidualEnergy16PerfectMatch — when c is the LS solution to wXX·c = wXx
// and wxx = c'·wXX·c, the residual should be near zero (≥1 due to clamp).
//
// Use a minimal 2×2 case we can compute by hand:
//
//	wXX = [[10, 0], [0, 10]] (Q0)
//	wXx = [3, 4]
//	c   = [3, 4] / 10 in some Q
//
// True residual energy = wxx - 2·wXx·c + c'·wXX·c = 25 - 50 + 25 = 0.
func TestResidualEnergy16ApproxZero(t *testing.T) {
	c := []int16{int16((3 << 14) / 10), int16((4 << 14) / 10)} // c in Q14
	wXX := []int32{10, 0, 0, 10}
	wXx := []int32{3, 4}
	wxx := int32(25)
	got := ResidualEnergy16Covar(c, wXX, wXx, wxx, 2, 14)
	// Should be near 0 — function clamps to ≥1.
	if got < 0 || got > 50 {
		t.Errorf("residual=%d, want near 0 (≤50)", got)
	}
}

// TestResidualEnergy16Saturation — extreme inputs don't crash.
func TestResidualEnergy16Saturation(t *testing.T) {
	c := []int16{1, 1, 1, 1}
	wXX := []int32{
		1000, 0, 0, 0,
		0, 1000, 0, 0,
		0, 0, 1000, 0,
		0, 0, 0, 1000,
	}
	wXx := []int32{0, 0, 0, 0}
	wxx := int32(100000)
	got := ResidualEnergy16Covar(c, wXX, wXx, wxx, 4, 14)
	if got < 0 {
		t.Errorf("got negative: %d", got)
	}
}
