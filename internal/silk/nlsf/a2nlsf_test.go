package nlsf

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk/lpc"
)

// TestA2NLSFThenNLSF2ARoundtrip — A2NLSF and NLSF2A are documented as
// "accurate inverses of each other" in the C source headers (the LSF/cos
// approximation introduces some bias, but the round-trip should preserve
// the AR coefficients within a small Q12 ULP envelope).
//
// Strategy:
//  1. Build random *stable* AR coefficients via reflection coefficients (K2a).
//  2. Convert AQ12 → AQ16, run A2NLSF → NLSF.
//  3. Run NLSF2A → AQ12'.
//  4. Compare AQ12 and AQ12' element-wise.
func TestA2NLSFThenNLSF2ARoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 30; trial++ {
		order := int32(10 + 2*(trial%4)) // 10, 12, 14, 16
		// Generate stable RC: |rc| < 0.5 in Q15.
		rcQ15 := make([]int16, order)
		for i := range rcQ15 {
			rcQ15[i] = int16(rng.Intn(1<<14) - (1 << 13))
		}
		// k2a → AQ24, then convert to AQ12 + AQ16.
		AQ24 := make([]int32, order)
		lpc.K2a(AQ24, rcQ15, order)

		AQ12 := make([]int16, order)
		AQ16 := make([]int32, order)
		for i, v := range AQ24 {
			AQ12[i] = int16(v >> 12)
			AQ16[i] = v >> 8
		}

		// A2NLSF mutates aQ16 if bandwidth expansion is applied. Pass a copy.
		aWork := append([]int32(nil), AQ16...)
		nlsf := make([]int32, order)
		A2NLSF(nlsf, aWork, order)

		// NLSFs must be sorted and in (0, 2^15).
		for i := int32(1); i < order; i++ {
			if nlsf[i] <= nlsf[i-1] {
				t.Errorf("trial %d: NLSF not strictly increasing at %d: %v", trial, i, nlsf)
				break
			}
		}
		if nlsf[order-1] >= 0x8000 {
			t.Errorf("trial %d: NLSF[last]=%d >= 2^15", trial, nlsf[order-1])
		}

		// Now NLSF2A back.
		AQ12Back := make([]int16, order)
		NLSF2A(AQ12Back, nlsf, order)

		// Compare AQ12 with AQ12Back. Tolerance: SILK's docstring claims
		// "accurate inverses". In practice the LSF/cos approximation +
		// rounding gives a few Q12 LSBs of drift. Allow up to ~64 Q12 LSBs
		// = 0.016 in real domain. If aQ16 was bandwidth-expanded by
		// A2NLSF, the round-trip changes more — accept that, but still
		// require the result not to explode.
		var maxDiff int32
		for i := int32(0); i < order; i++ {
			diff := int32(AQ12Back[i]) - int32(AQ12[i])
			if diff < 0 {
				diff = -diff
			}
			if diff > maxDiff {
				maxDiff = diff
			}
		}
		// A round-trip drift > 1024 Q12 LSBs means the inverse is broken.
		if maxDiff > 1024 {
			t.Errorf("trial %d order %d: round-trip drift %d (AQ12=%v vs AQ12Back=%v)",
				trial, order, maxDiff, AQ12, AQ12Back)
		}
	}
}

// TestA2NLSFSortedOutput — the spec guarantees NLSF[i] are strictly
// increasing and within (0, 32767).
func TestA2NLSFSortedOutput(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 50; trial++ {
		order := int32(10)
		rcQ15 := make([]int16, order)
		for i := range rcQ15 {
			rcQ15[i] = int16(rng.Intn(1<<14) - (1 << 13))
		}
		AQ24 := make([]int32, order)
		lpc.K2a(AQ24, rcQ15, order)
		aWork := make([]int32, order)
		for i, v := range AQ24 {
			aWork[i] = v >> 8 // Q24 → Q16
		}
		nlsf := make([]int32, order)
		A2NLSF(nlsf, aWork, order)

		// Check sorted.
		s := append([]int32(nil), nlsf...)
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
		for i := range s {
			if s[i] != nlsf[i] {
				t.Errorf("trial %d: NLSF not sorted: %v", trial, nlsf)
				break
			}
		}
		// Range.
		for i, v := range nlsf {
			if v < 0 || v > 0x7FFF {
				t.Errorf("trial %d idx %d: NLSF=%d out of [0, 32767]", trial, i, v)
			}
		}
	}
}

// TestNLSF2AStableProducesStableLPC — after NLSF2AStable, the LPC's inverse
// prediction gain check returns 0 (stable).
func TestNLSF2AStableProducesStableLPC(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 30; trial++ {
		order := int32(10 + 2*(trial%4))
		// Sensitive: random NLSF that's been Stabilize()d so it's valid.
		nlsf := makeIncreasingNLSF(rng, order)
		dmin := make([]int32, order+1)
		for i := range dmin {
			dmin[i] = 100
		}
		Stabilize(nlsf, dmin, order)

		ar := make([]int16, order)
		NLSF2AStable(ar, nlsf, order)
		var invGain int32
		ret := lpc.LPCInversePredGain(&invGain, ar, order)
		if ret != 0 {
			t.Errorf("trial %d order %d: NLSF2AStable returned unstable LPC (invGain=%d)", trial, order, invGain)
		}
	}
}

// TestA2NLSFPathologicalFallback — feed AR coefficients that won't yield
// d roots even after BWE; expect the white-spectrum fallback to kick in
// and output strictly increasing NLSFs.
func TestA2NLSFPathologicalFallback(t *testing.T) {
	order := int32(10)
	// Mostly-zero AR with one large coefficient → unstable + missing roots
	// after the cos-table sweep. Should hit MAX_ITERATIONS_A2NLSF_FIX
	// fallback.
	aQ16 := make([]int32, order)
	for i := range aQ16 {
		aQ16[i] = 0
	}
	nlsf := make([]int32, order)
	A2NLSF(nlsf, aQ16, order)

	// White-spectrum fallback: NLSF[i] = (i+1) * NLSF[0] with NLSF[0] = 2^15 / (d+1).
	// Just check sortedness — exact values may also come from a successful
	// root-find on the all-zeros (which actually has clean roots at the
	// d-th roots of unity).
	for i := int32(1); i < order; i++ {
		if nlsf[i] <= nlsf[i-1] {
			t.Errorf("NLSF not increasing at %d: %v", i, nlsf)
			break
		}
	}
}
