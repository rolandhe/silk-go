package ltp

import (
	"math"
	"math/rand"
	"testing"
)

// TestFindLTPSmokeTest — drive find_LTP with a simple periodic signal and
// verify it doesn't crash, produces valid corrRshifts, and writes
// reasonable LTP coefficients.
func TestFindLTPSmokeTest(t *testing.T) {
	subfrLength := int32(40)
	memOffset := int32(60) // ≥ max(lag) + LTPOrder/2
	pitch := int32(50)
	lag := []int32{pitch, pitch, pitch, pitch}

	// Build a periodic residual signal of total length = memOffset + 2 subframes.
	totalLen := memOffset + 2*subfrLength + 16
	rFirst := make([]int16, totalLen)
	rLast := make([]int16, totalLen)
	for i := int32(0); i < totalLen; i++ {
		v := math.Sin(2*math.Pi*float64(i)/float64(pitch)) * 5000
		rFirst[i] = int16(v)
		rLast[i] = int16(v)
	}

	wghtQ15 := []int32{1 << 14, 1 << 14, 1 << 14, 1 << 14} // 0.5 each
	bQ14 := make([]int16, NBSubFr*LTPOrder)
	WLTP := make([]int32, NBSubFr*LTPOrder*LTPOrder)
	corrRshifts := make([]int32, NBSubFr)
	var ltpredCodGainQ7 int32

	FindLTP(bQ14, WLTP, &ltpredCodGainQ7, rFirst, rLast, lag, wghtQ15,
		subfrLength, memOffset, corrRshifts)

	// Sanity checks.
	for k := int32(0); k < NBSubFr; k++ {
		if corrRshifts[k] < -32 || corrRshifts[k] > 32 {
			t.Errorf("subfr %d: corrRshifts=%d out of plausible range", k, corrRshifts[k])
		}
	}
	for i, v := range bQ14 {
		// LTP coefs are clamped to [-16000, 28000] in Q14 (= ~[-1, +1.7]).
		if int32(v) < -16000 || int32(v) > 28000 {
			t.Errorf("bQ14[%d]=%d out of [-16000, 28000]", i, v)
		}
	}
	// LTP coding gain should be non-negative for a periodic signal that
	// matches the lag (LTP can predict it well → res nrg < input nrg →
	// log ratio > 0).
	if ltpredCodGainQ7 < 0 {
		t.Errorf("ltpredCodGainQ7=%d < 0 for periodic signal", ltpredCodGainQ7)
	}
}

// TestFindLTPRandomNoCrash — random inputs don't blow up the algorithm.
func TestFindLTPRandomNoCrash(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	subfrLength := int32(40)
	memOffset := int32(80)
	totalLen := memOffset + NBSubFr*subfrLength + 16

	for trial := 0; trial < 5; trial++ {
		rFirst := make([]int16, totalLen)
		rLast := make([]int16, totalLen)
		for i := range rFirst {
			rFirst[i] = int16(rng.Intn(20001) - 10000)
			rLast[i] = int16(rng.Intn(20001) - 10000)
		}
		// pitch in [40, 60], well within memOffset budget.
		lag := []int32{
			int32(40 + rng.Intn(21)),
			int32(40 + rng.Intn(21)),
			int32(40 + rng.Intn(21)),
			int32(40 + rng.Intn(21)),
		}
		wghtQ15 := []int32{
			int32(rng.Intn(16384) + 100),
			int32(rng.Intn(16384) + 100),
			int32(rng.Intn(16384) + 100),
			int32(rng.Intn(16384) + 100),
		}
		bQ14 := make([]int16, NBSubFr*LTPOrder)
		WLTP := make([]int32, NBSubFr*LTPOrder*LTPOrder)
		corrRshifts := make([]int32, NBSubFr)

		FindLTP(bQ14, WLTP, nil, rFirst, rLast, lag, wghtQ15,
			subfrLength, memOffset, corrRshifts)

		for i, v := range bQ14 {
			if int32(v) < -16000 || int32(v) > 28000 {
				t.Errorf("trial %d bQ14[%d]=%d out of [-16000, 28000]",
					trial, i, v)
			}
		}
	}
}

// TestFindLTPNilCodingGain — passing nil for ltpredCodGainQ7 must not panic.
func TestFindLTPNilCodingGain(t *testing.T) {
	subfrLength := int32(40)
	memOffset := int32(60)
	totalLen := memOffset + NBSubFr*subfrLength + 16
	rFirst := make([]int16, totalLen)
	rLast := make([]int16, totalLen)
	for i := range rFirst {
		rFirst[i] = int16(i % 100)
		rLast[i] = int16(i % 100)
	}
	lag := []int32{50, 50, 50, 50}
	wghtQ15 := []int32{16384, 16384, 16384, 16384}
	bQ14 := make([]int16, NBSubFr*LTPOrder)
	WLTP := make([]int32, NBSubFr*LTPOrder*LTPOrder)
	corrRshifts := make([]int32, NBSubFr)
	FindLTP(bQ14, WLTP, nil, rFirst, rLast, lag, wghtQ15,
		subfrLength, memOffset, corrRshifts)
}
