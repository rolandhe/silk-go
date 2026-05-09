package ltp

import (
	"testing"

	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// TestQuantGainsPicksValidIndices — quantization should produce indices in
// the right ranges for the chosen periodicity, and BQ14 should match the
// chosen codebook entries afterward.
func TestQuantGainsPicksValidIndices(t *testing.T) {
	// Use a row from codebook 1 as input — the function should pick that
	// codebook (or another with smaller RD) and return a matching entry.
	BQ14 := make([]int16, NBSubFr*LTPOrder)
	cb1 := tables.LTPVQPtrsQ14[1]
	for j := int32(0); j < NBSubFr; j++ {
		// Use entry 5 of cb1 for every subframe.
		for k := int32(0); k < LTPOrder; k++ {
			BQ14[j*LTPOrder+k] = cb1[5*LTPOrder+k]
		}
	}

	// Identity-ish W matrix per subframe.
	WQ18 := make([]int32, NBSubFr*LTPOrder*LTPOrder)
	for j := int32(0); j < NBSubFr; j++ {
		for i := int32(0); i < LTPOrder; i++ {
			WQ18[j*LTPOrder*LTPOrder+i*LTPOrder+i] = 1 << 18
		}
	}

	cbkIndex := make([]int32, NBSubFr)
	var periodicityIndex int32

	QuantGains(BQ14, cbkIndex, &periodicityIndex, WQ18, 16, 0)

	if periodicityIndex < 0 || periodicityIndex > 2 {
		t.Errorf("periodicityIndex=%d out of [0, 2]", periodicityIndex)
	}
	chosenCB := tables.LTPVQPtrsQ14[periodicityIndex]
	maxIdx := tables.LTP_vq_sizes[periodicityIndex]
	for j := int32(0); j < NBSubFr; j++ {
		if cbkIndex[j] < 0 || cbkIndex[j] >= maxIdx {
			t.Errorf("subfr %d: cbkIndex=%d out of [0, %d)",
				j, cbkIndex[j], maxIdx)
		}
		// BQ14 must match the chosen codebook row.
		row := cbkIndex[j] * LTPOrder
		for k := int32(0); k < LTPOrder; k++ {
			if BQ14[j*LTPOrder+k] != chosenCB[row+k] {
				t.Errorf("subfr %d k %d: BQ14=%d codebook=%d",
					j, k, BQ14[j*LTPOrder+k], chosenCB[row+k])
			}
		}
	}
}

// TestQuantGainsLowComplexityShortcut — lowComplexity=1 should not produce
// worse results than lowComplexity=0 (it may pick a different codebook,
// but both should be valid quantizations).
func TestQuantGainsLowComplexityValid(t *testing.T) {
	rng := newDeterministicRNG(7)
	BQ14a := make([]int16, NBSubFr*LTPOrder)
	BQ14b := make([]int16, NBSubFr*LTPOrder)
	for i := range BQ14a {
		v := int16(rng.intn(20001) - 10000)
		BQ14a[i] = v
		BQ14b[i] = v
	}
	WQ18 := make([]int32, NBSubFr*LTPOrder*LTPOrder)
	for j := int32(0); j < NBSubFr; j++ {
		for i := int32(0); i < LTPOrder; i++ {
			WQ18[j*LTPOrder*LTPOrder+i*LTPOrder+i] = 1 << 18
		}
	}
	idxA := make([]int32, NBSubFr)
	idxB := make([]int32, NBSubFr)
	var pA, pB int32
	QuantGains(BQ14a, idxA, &pA, WQ18, 16, 0) // full search
	QuantGains(BQ14b, idxB, &pB, WQ18, 16, 1) // low-complexity

	for j := int32(0); j < NBSubFr; j++ {
		if idxA[j] < 0 || idxA[j] >= tables.LTP_vq_sizes[pA] {
			t.Errorf("full-search idxA[%d]=%d out of range", j, idxA[j])
		}
		if idxB[j] < 0 || idxB[j] >= tables.LTP_vq_sizes[pB] {
			t.Errorf("low-complexity idxB[%d]=%d out of range", j, idxB[j])
		}
	}
}

// Tiny deterministic RNG to avoid pulling rand into more imports.
type detRNG struct{ s uint64 }

func newDeterministicRNG(seed uint64) *detRNG { return &detRNG{s: seed*2862933555777941757 + 1} }
func (r *detRNG) intn(n int) int {
	r.s = r.s*6364136223846793005 + 1442695040888963407
	return int(r.s>>33) % n
}
