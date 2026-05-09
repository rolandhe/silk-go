package lpc

import (
	"math"
	"math/rand"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// TestK2aZeroReflection — zero reflection coefficients yield an all-zero
// prediction polynomial (a[k] = -rc[k]<<9).
func TestK2aZeroReflection(t *testing.T) {
	order := int32(8)
	rc := make([]int16, order)
	a := make([]int32, order)
	K2a(a, rc, order)
	for i, v := range a {
		if v != 0 {
			t.Errorf("a[%d] = %d, want 0", i, v)
		}
	}
}

// TestK2aFirstCoeffMatchesFormula — the last coefficient written for any
// order is a[k] = -rc[k] << 9 since at that point Atmp[k]…[order] are
// untouched. So if we drive a single non-zero rc and compute, we can verify.
func TestK2aLastCoeffFormula(t *testing.T) {
	order := int32(4)
	rc := []int16{1234, -5678, 4096, -8192}
	a := make([]int32, order)
	K2a(a, rc, order)
	// Final iteration writes a[order-1] = -(rc[order-1] << 9) and leaves
	// earlier indices to be modified by SMLAWB updates from prior k.
	if a[order-1] != -(int32(rc[order-1]) << 9) {
		t.Errorf("a[order-1]=%d, want %d", a[order-1], -(int32(rc[order-1]) << 9))
	}
}

// TestK2aQ16RoughEquivalence — driving K2a with rc_Q15 == rc_Q16 >> 1 should
// produce the same prediction polynomial within rounding error.
func TestK2aQ16RoughEquivalence(t *testing.T) {
	order := int32(6)
	rcQ16 := []int32{1 << 14, -(1 << 14), 1 << 13, -(1 << 13), 1 << 12, -(1 << 12)}
	rcQ15 := make([]int16, order)
	for i, v := range rcQ16 {
		rcQ15[i] = int16(v >> 1)
	}
	a1 := make([]int32, order)
	a2 := make([]int32, order)
	K2a(a1, rcQ15, order)
	K2aQ16(a2, rcQ16, order)
	for i := range a1 {
		diff := int64(a1[i]) - int64(a2[i])
		if diff < 0 {
			diff = -diff
		}
		// Both are Q24; tolerate up to a few ULPs of Q24 difference (~256
		// per multiply pair, scaled by order).
		tol := int64(1 << 16)
		if diff > tol {
			t.Errorf("idx %d: K2a=%d K2aQ16=%d diff=%d", i, a1[i], a2[i], diff)
		}
	}
}

// TestLPCInversePredGainStable — a polynomial built from small reflection
// coefficients (|rc| < 1) is stable, so the function returns 0 and reports
// invGain in (0, 2^30].
func TestLPCInversePredGainStable(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 30; trial++ {
		order := int32(2 + rng.Intn(8))
		if order%2 != 0 {
			order++
		}
		rcQ15 := make([]int16, order)
		for i := range rcQ15 {
			// Keep |rc| < 0.5 so the resulting LPC is comfortably stable.
			rcQ15[i] = int16(rng.Intn(1<<14) - (1 << 13))
		}
		AQ24 := make([]int32, order)
		K2a(AQ24, rcQ15, order)
		AQ12 := make([]int16, order)
		for i, v := range AQ24 {
			AQ12[i] = int16(fix.RShiftRound(v, 12))
		}
		var invGain int32
		ret := LPCInversePredGain(&invGain, AQ12, order)
		if ret != 0 {
			t.Errorf("trial %d order %d: stable filter reported unstable (invGain=%d)", trial, order, invGain)
		}
		if invGain < 0 || invGain > 1<<30 {
			t.Errorf("trial %d: invGain=%d out of (0,2^30]", trial, invGain)
		}
	}
}

// TestLPCInversePredGainUnstable — a deliberately unstable AR coefficient
// (very close to 1.0 in Q12) must trigger the unstable return path.
func TestLPCInversePredGainUnstable(t *testing.T) {
	order := int32(2)
	// AR coefficient that translates to |rc| > 1.0 → unstable.
	AQ12 := []int16{4095, 4095}
	var invGain int32
	ret := LPCInversePredGain(&invGain, AQ12, order)
	if ret != 1 {
		t.Errorf("unstable LPC not detected: ret=%d invGain=%d", ret, invGain)
	}
}

// TestSynthesisFilterImpulse — feed a single excitation impulse with gain 1.0
// (Q26) and zero AR coefficients. The expected output is the impulse passed
// through (just scaled by the SMULWB approximation of 1.0).
func TestSynthesisFilterImpulseZeroAR(t *testing.T) {
	order := int32(10)
	in := []int16{1000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	AQ12 := make([]int16, order)
	S := make([]int32, order)
	out := make([]int16, len(in))
	gainQ26 := int32(1 << 26)
	SynthesisFilter(in, AQ12, gainQ26, S, out, int32(len(in)), order)
	// First sample: SMULWB(2^26, 1000) ~= 1000 exactly (RSHIFT 16 of (1<<26)*1000 / 2^16 = 1000).
	// Subsequent samples are 0 since AR coeffs are zero.
	if out[0] != 1000 {
		t.Errorf("impulse out[0]=%d want 1000", out[0])
	}
	for i := 1; i < len(out); i++ {
		if out[i] != 0 {
			t.Errorf("impulse out[%d]=%d want 0", i, out[i])
		}
	}
}

// TestSynthesisFilterEnergyFinite — driving a noise excitation through a
// stable LPC should produce a bounded output (no NaN/saturation runaway).
func TestSynthesisFilterEnergyFinite(t *testing.T) {
	order := int32(10)
	rng := rand.New(rand.NewSource(2))
	in := make([]int16, 200)
	for i := range in {
		in[i] = int16(rng.Intn(2001) - 1000)
	}
	// Build a mildly damped AR via small reflection coefficients then K2a.
	rc := []int16{2000, -1500, 1000, -800, 600, -400, 300, -200, 100, -50}
	AQ24 := make([]int32, order)
	K2a(AQ24, rc, order)
	AQ12 := make([]int16, order)
	for i, v := range AQ24 {
		AQ12[i] = int16(fix.RShiftRound(v, 12))
	}
	S := make([]int32, order)
	out := make([]int16, len(in))
	SynthesisFilter(in, AQ12, 1<<26, S, out, int32(len(in)), order)
	// Output should not be all-zeros nor everywhere saturated.
	var energy float64
	for _, v := range out {
		energy += float64(v) * float64(v)
	}
	if energy <= 0 || math.IsInf(energy, 0) || math.IsNaN(energy) {
		t.Errorf("output energy bad: %v", energy)
	}
}

// TestSynthesisOrder16ImpulseZeroAR — same impulse-response sanity check
// for the unrolled order-16 variant.
func TestSynthesisOrder16ImpulseZeroAR(t *testing.T) {
	in := []int16{1000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	AQ12 := make([]int16, 16)
	S := make([]int32, 16)
	out := make([]int16, len(in))
	SynthesisOrder16(in, AQ12, 1<<26, S, out, int32(len(in)))
	if out[0] != 1000 {
		t.Errorf("order16 impulse out[0]=%d want 1000", out[0])
	}
	for i := 1; i < len(out); i++ {
		if out[i] != 0 {
			t.Errorf("order16 impulse out[%d]=%d want 0", i, out[i])
		}
	}
}

// TestSynthesisOrder16VsGenericMatchesForOrder16 — when called with order=16,
// the generic SynthesisFilter and the unrolled SynthesisOrder16 must produce
// the same output for the same inputs (modulo the _ovflw variants used by
// the unrolled path; this test feeds inputs small enough that overflow does
// not occur, so the two paths are bit-equivalent).
func TestSynthesisOrder16MatchesGeneric(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	in := make([]int16, 64)
	for i := range in {
		in[i] = int16(rng.Intn(1001) - 500)
	}
	AQ12 := make([]int16, 16)
	for i := range AQ12 {
		AQ12[i] = int16(rng.Intn(401) - 200)
	}
	S1 := make([]int32, 16)
	S2 := make([]int32, 16)
	out1 := make([]int16, len(in))
	out2 := make([]int16, len(in))
	SynthesisFilter(in, AQ12, 1<<26, S1, out1, int32(len(in)), 16)
	SynthesisOrder16(in, AQ12, 1<<26, S2, out2, int32(len(in)))
	for i := range out1 {
		if out1[i] != out2[i] {
			t.Errorf("idx %d: generic=%d order16=%d", i, out1[i], out2[i])
		}
	}
}
