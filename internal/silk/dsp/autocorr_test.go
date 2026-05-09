package dsp

import (
	"math"
	"math/rand"
	"testing"
)

// TestAutocorrFloatReference — autocorr should match a float64 reference at
// every lag, after re-scaling by the recorded shift.
func TestAutocorrFloatReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 20; trial++ {
		n := int32(64 + rng.Intn(192))
		corrCount := int32(rng.Intn(int(n-1)) + 1)
		x := make([]int16, n)
		for i := range x {
			x[i] = int16(rng.Intn(20001) - 10000)
		}

		results := make([]int32, corrCount)
		var scale int32
		Autocorr(results, &scale, x, n, corrCount)

		// Float reference. The C function adds 1 to lag-0 to handle all-zero
		// inputs; our Go port mirrors that, so the reference must too.
		var ref0 float64
		for i := int32(0); i < n; i++ {
			ref0 += float64(x[i]) * float64(x[i])
		}
		ref0 += 1

		// Re-scale: nRightShifts > 0 means results = ref >> shift; <0 means results = ref << -shift.
		var got0 float64
		if scale >= 0 {
			got0 = float64(results[0]) * math.Pow(2, float64(scale))
		} else {
			got0 = float64(results[0]) * math.Pow(2, float64(scale))
		}
		// Allow ~1 LSB at the working scale.
		tol := math.Max(math.Abs(ref0)*1e-6, math.Pow(2, float64(scale))*2)
		if math.Abs(got0-ref0) > tol {
			t.Errorf("trial %d lag 0: got %v ref %v scale %d", trial, got0, ref0, scale)
		}

		// Higher lags.
		for k := int32(1); k < corrCount; k++ {
			var refK float64
			for i := int32(0); i+k < n; i++ {
				refK += float64(x[i]) * float64(x[i+k])
			}
			gotK := float64(results[k]) * math.Pow(2, float64(scale))
			tol := math.Max(math.Abs(refK)*1e-5+1, math.Pow(2, float64(scale))*2)
			if math.Abs(gotK-refK) > tol {
				t.Errorf("trial %d lag %d: got %v ref %v scale %d", trial, k, gotK, refK, scale)
			}
		}
	}
}

// TestAutocorrZeroInput — all-zero input must not panic; lag-0 result is 1
// (because the C source adds +1 to handle that case).
func TestAutocorrZeroInput(t *testing.T) {
	x := make([]int16, 32)
	results := make([]int32, 4)
	var scale int32
	Autocorr(results, &scale, x, int32(len(x)), 4)
	// Lag 0 should be exactly 1 (or close to it after scale normalization).
	rescaled := float64(results[0]) * math.Pow(2, float64(scale))
	if math.Abs(rescaled-1) > 0.5 {
		t.Errorf("zero input lag 0 rescaled = %v, want ~1", rescaled)
	}
}
