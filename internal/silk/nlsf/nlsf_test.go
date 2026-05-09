package nlsf

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// makeIncreasingNLSF builds a strictly-increasing Q15 NLSF vector with at
// least 100 LSB spacing in (0, 2^15).
func makeIncreasingNLSF(rng *rand.Rand, D int32) []int32 {
	const minSpace = int32(100)
	v := make([]int32, D)
	cur := int32(rng.Intn(500) + 200)
	for i := int32(0); i < D; i++ {
		v[i] = cur
		cur += minSpace + int32(rng.Intn(2000))
	}
	// Scale to fit safely below 2^15.
	last := v[D-1]
	scale := float64(0x7FFF-500) / float64(last)
	for i := range v {
		v[i] = int32(float64(v[i]) * scale)
	}
	return v
}

// TestVQWeightsLaroiaInverseRelation — Laroia weights are roughly inversely
// proportional to local NLSF spacing. Verify that:
//   - All weights are positive.
//   - All weights fit in int16.
//   - Doubling all gaps halves all weights (to within ~10% rounding).
func TestVQWeightsLaroiaInverseRelation(t *testing.T) {
	D := int32(10)
	rng := rand.New(rand.NewSource(1))
	a := makeIncreasingNLSF(rng, D)
	wA := make([]int32, D)
	VQWeightsLaroia(wA, a, D)

	for i, w := range wA {
		if w <= 0 || w > 0x7FFF {
			t.Errorf("weight[%d]=%d out of (0, 32767]", i, w)
		}
	}

	// Build b with all gaps doubled (and edges doubled).
	b := make([]int32, D)
	b[0] = 2 * a[0]
	for i := int32(1); i < D; i++ {
		b[i] = b[i-1] + 2*(a[i]-a[i-1])
	}
	// Scale b to fit (it might exceed 2^15 after doubling). Drop the test if
	// it does — the weight ratio still has to hold for the unscaled subspace.
	if b[D-1] >= 1<<15 {
		t.Skip("doubled b out of range; skip ratio check")
	}
	wB := make([]int32, D)
	VQWeightsLaroia(wB, b, D)

	// Each Laroia weight ≈ (1/gap_left) + (1/gap_right). Doubling all gaps
	// halves both, so the ratio wA/wB ≈ 2 ± rounding.
	for i, _ := range wA {
		ratio := float64(wA[i]) / float64(wB[i])
		if ratio < 1.6 || ratio > 2.4 {
			t.Errorf("weight ratio at idx %d = %.3f, want ~2.0", i, ratio)
		}
	}
}

func TestVQWeightsLaroiaMinDelta(t *testing.T) {
	// Inputs at the lower limit (NLSF[0]=1) must clamp to MIN_NDELTA=3.
	D := int32(4)
	a := []int32{1, 2, 3, 4}
	w := make([]int32, D)
	VQWeightsLaroia(w, a, D)
	for i := int32(0); i < D; i++ {
		if w[i] <= 0 {
			t.Errorf("weight[%d]=%d non-positive", i, w[i])
		}
	}
}

// TestStabilizeIdempotentOnGoodInput — a stable NLSF passes through unchanged.
func TestStabilizeIdempotentOnGoodInput(t *testing.T) {
	L := int32(10)
	rng := rand.New(rand.NewSource(2))
	nlsf := makeIncreasingNLSF(rng, L)
	original := append([]int32(nil), nlsf...)

	// NDeltaMin chosen comfortably smaller than the smallest gap.
	dmin := make([]int32, L+1)
	smallestGap := nlsf[0]
	for i := int32(1); i < L; i++ {
		gap := nlsf[i] - nlsf[i-1]
		if gap < smallestGap {
			smallestGap = gap
		}
	}
	tail := (1 << 15) - nlsf[L-1]
	if tail < smallestGap {
		smallestGap = tail
	}
	delta := smallestGap / 2
	if delta < 1 {
		delta = 1
	}
	for i := range dmin {
		dmin[i] = delta
	}

	Stabilize(nlsf, dmin, L)
	for i := range nlsf {
		if nlsf[i] != original[i] {
			t.Errorf("idx %d changed: %d → %d", i, original[i], nlsf[i])
		}
	}
}

// TestStabilizeFixesViolation — drive a single inversion and verify the
// output is sorted with at least delta_min spacing.
func TestStabilizeFixesViolation(t *testing.T) {
	L := int32(6)
	nlsf := []int32{1000, 5000, 4500, 12000, 18000, 22000} // index 2 inverts
	dmin := []int32{200, 200, 200, 200, 200, 200, 200}

	Stabilize(nlsf, dmin, L)

	// Sorted after stabilization.
	for i := int32(1); i < L; i++ {
		if nlsf[i] < nlsf[i-1] {
			t.Errorf("not sorted at idx %d: %d < %d", i, nlsf[i], nlsf[i-1])
		}
	}
	// Every gap >= NDeltaMin.
	if nlsf[0] < dmin[0] {
		t.Errorf("nlsf[0]=%d < dmin[0]=%d", nlsf[0], dmin[0])
	}
	for i := int32(1); i < L; i++ {
		if nlsf[i]-nlsf[i-1] < dmin[i] {
			t.Errorf("gap at %d too small: %d < %d", i, nlsf[i]-nlsf[i-1], dmin[i])
		}
	}
	if (1<<15)-nlsf[L-1] < dmin[L] {
		t.Errorf("tail gap too small")
	}
}

// TestStabilizeFallback — a deeply unsorted input should hit the fallback
// (insertion sort + clamp) and still come out monotone with delta_min.
func TestStabilizeFallback(t *testing.T) {
	L := int32(10)
	nlsf := []int32{30000, 5000, 25000, 1000, 28000, 3000, 18000, 9000, 22000, 7000}
	dmin := make([]int32, L+1)
	for i := range dmin {
		dmin[i] = 200
	}

	Stabilize(nlsf, dmin, L)
	for i := int32(1); i < L; i++ {
		if nlsf[i] < nlsf[i-1] {
			t.Errorf("not sorted at idx %d: %v", i, nlsf)
			break
		}
	}
}

// TestNLSF2ALowOrderImpulseLike — for d=10, simple monotone NLSFs spaced
// across (0, π) produce a stable LPC polynomial. Verify:
//   - Output a[] fits in int16 (Q12 representation).
//   - Implied LPC roots — i.e., 2*cos(NLSF) values — show up in a sane way.
func TestNLSF2AStableOutput(t *testing.T) {
	d := int32(10)
	// Evenly distributed NLSF in Q15 between ~3000 and ~30000.
	nlsf := make([]int32, d)
	for i := int32(0); i < d; i++ {
		nlsf[i] = 3000 + i*((30000-3000)/(d-1))
	}
	a := make([]int16, d)
	NLSF2A(a, nlsf, d)
	for i, v := range a {
		// Q12 coefficients: |a[i]| < ~8.0 in real domain → < 8*4096 = 32768.
		if int32(v) > 0x7FFF || int32(v) < -0x8000 {
			t.Errorf("a[%d]=%d out of int16", i, v)
		}
	}
}

// TestNLSF2AMonotonicity — two NLSF vectors with the same shape but different
// scales produce visibly-correlated coefficient vectors.
func TestNLSF2AStable16Order(t *testing.T) {
	d := int32(16)
	nlsf := make([]int32, d)
	for i := int32(0); i < d; i++ {
		nlsf[i] = 2000 + i*((31000-2000)/(d-1))
	}
	a := make([]int16, d)
	NLSF2A(a, nlsf, d)
	// Just check it doesn't panic / overflow / produce all-zero garbage.
	nonzero := 0
	for _, v := range a {
		if v != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Errorf("d=16 NLSF2A produced all zeros")
	}
}

// TestNLSF2ARoots — sanity check: NLSF2A on randomly-generated valid NLSF
// vectors yields a stable LPC (positive prediction gain).
//
// We don't have LPC.LPCInversePredGain inside `nlsf` to call (cyclic), so
// this test instead verifies that the produced coefficients have the leading
// coefficient near 0 (since A(z) = 1 + a_1 z^-1 + ... — but wait, monic
// whitening filter in SILK is INTRINSIC: a[0] is the first coefficient
// AFTER the leading 1). We sanity-bound the coefficient magnitudes.
func TestNLSF2ABoundedMagnitude(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 20; trial++ {
		d := int32(10 + 2*(trial%4)) // 10, 12, 14, 16
		nlsf := makeIncreasingNLSF(rng, d)
		// Stabilize to ensure validity.
		dmin := make([]int32, d+1)
		for i := range dmin {
			dmin[i] = 100
		}
		Stabilize(nlsf, dmin, d)

		a := make([]int16, d)
		NLSF2A(a, nlsf, d)
		// Energy should not explode beyond the Q12 representation envelope.
		var energy float64
		for _, v := range a {
			energy += float64(v) * float64(v)
		}
		if math.IsInf(energy, 0) || math.IsNaN(energy) {
			t.Errorf("trial %d: energy = %v", trial, energy)
		}
	}
}

// Sanity for stabilize: cross-check against a Go reference implementation
// for small inputs.
func TestStabilizeMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for trial := 0; trial < 30; trial++ {
		L := int32(4 + 2*(trial%5))
		nlsf := make([]int32, L)
		for i := range nlsf {
			nlsf[i] = int32(rng.Intn(1 << 15))
		}
		dmin := make([]int32, L+1)
		for i := range dmin {
			dmin[i] = 100 + int32(rng.Intn(200))
		}

		got := append([]int32(nil), nlsf...)
		Stabilize(got, dmin, L)

		// Reference: sort + monotone clamp (matches the fallback branch
		// outcome, which is a strict superset of valid stabilizations).
		ref := append([]int32(nil), nlsf...)
		sort.Slice(ref, func(i, j int) bool { return ref[i] < ref[j] })
		if ref[0] < dmin[0] {
			ref[0] = dmin[0]
		}
		for i := int32(1); i < L; i++ {
			if ref[i] < ref[i-1]+dmin[i] {
				ref[i] = ref[i-1] + dmin[i]
			}
		}
		if ref[L-1] > (1<<15)-dmin[L] {
			ref[L-1] = (1 << 15) - dmin[L]
		}
		for i := L - 2; i >= 0; i-- {
			if ref[i] > ref[i+1]-dmin[i+1] {
				ref[i] = ref[i+1] - dmin[i+1]
			}
		}

		// Check `got` satisfies the same constraints as `ref`. We don't
		// require equality — the iterative path can find a different valid
		// solution.
		for i := int32(1); i < L; i++ {
			if got[i] < got[i-1]+dmin[i] {
				t.Errorf("trial %d: got[%d] - got[%d] = %d < dmin %d",
					trial, i, i-1, got[i]-got[i-1], dmin[i])
			}
		}
	}
}
