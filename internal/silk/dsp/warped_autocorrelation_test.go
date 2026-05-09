package dsp

import (
	"math"
	"math/rand"
	"testing"
)

// TestWarpedAutocorrelationOutputBounds — the C source asserts that
// scale ∈ [-30, 12] and corr_QC[0] ≥ 0. Verify our port maintains those.
func TestWarpedAutocorrelationOutputBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 30; trial++ {
		length := int32(64 + rng.Intn(200))
		order := int32(2 + 2*(rng.Intn(8))) // even, 2..16
		x := make([]int16, length)
		for i := range x {
			x[i] = int16(rng.Intn(20001) - 10000)
		}
		corr := make([]int32, order+1)
		var scale int32
		warpingQ16 := int16(rng.Intn(20001) - 10000) // ±0.15 in Q16-ish
		WarpedAutocorrelation(corr, &scale, x, warpingQ16, length, order)
		if scale < -30 || scale > 12 {
			t.Errorf("trial %d: scale=%d out of [-30, 12]", trial, scale)
		}
		if corr[0] < 0 {
			t.Errorf("trial %d: corr[0]=%d < 0", trial, corr[0])
		}
	}
}

// TestWarpedAutocorrelationZeroWarpingMatchesPlain — when warping = 0, the
// all-pass sections collapse to a delay line. The result should match
// the plain autocorrelation up to scaling.
func TestWarpedAutocorrelationZeroWarping(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	length := int32(128)
	order := int32(8)
	x := make([]int16, length)
	for i := range x {
		x[i] = int16(rng.Intn(2001) - 1000)
	}

	corr := make([]int32, order+1)
	var scale int32
	WarpedAutocorrelation(corr, &scale, x, 0, length, order)

	// Plain autocorrelation reference.
	plainCorr := make([]int32, order+1)
	var plainScale int32
	Autocorr(plainCorr, &plainScale, x, length, order+1)

	// Both vectors are scaled differently. Compute the ratio in float and
	// verify shape (lag-1/lag-0, etc.) matches within ~5%.
	for k := int32(1); k <= order; k++ {
		ratioWarped := float64(corr[k]) / float64(corr[0])
		ratioPlain := float64(plainCorr[k]) / float64(plainCorr[0])
		diff := math.Abs(ratioWarped - ratioPlain)
		// Some tolerance because the warping=0 path through allpass still
		// has a tiny phase response near DC. 0.05 absolute is generous.
		if diff > 0.05 {
			t.Errorf("lag %d: warped ratio=%.4f plain ratio=%.4f (diff=%.4f)",
				k, ratioWarped, ratioPlain, diff)
		}
	}
}

// TestWarpedAutocorrelationZeroInput — all-zero input must not panic and
// produces zero correlation.
func TestWarpedAutocorrelationZeroInput(t *testing.T) {
	x := make([]int16, 64)
	corr := make([]int32, 9)
	var scale int32
	WarpedAutocorrelation(corr, &scale, x, 1000, 64, 8)
	for i, v := range corr {
		if v != 0 {
			t.Errorf("corr[%d]=%d, want 0", i, v)
		}
	}
	// scale falls to its lower clamp -22 (since corrQC[0]=0 → CLZ=64 → lsh=29 clamped to 30-QC=20).
	// Actually CLZ64(0) is 64; lsh = 64 - 35 = 29, clamped to 20. scale = -(10 + 20) = -30.
	if scale != -30 {
		t.Errorf("zero-input scale=%d, want -30", scale)
	}
}
