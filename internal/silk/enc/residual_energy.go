package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// ResidualEnergyFIX — SKP_Silk_residual_energy_FIX.
//
// Per-subframe residual energy. Runs LPC analysis on each half frame
// (using the matching aQ12 set), measures the per-subframe sum-of-squares,
// then scales by the squared per-subframe gain. The energies and their
// Q-domains come back through the parallel `nrgs` and `nrgsQ` arrays.
//
// All four subframes share the same `subfr_length` and each subframe is
// preceded by `LPC_order` history samples (so the input buffer is laid
// out as 4 × (LPC_order + subfr_length) samples).
//
// Translated from vendor/silk/src/SKP_Silk_residual_energy_FIX.c.
func ResidualEnergyFIX(
	nrgs []int32,
	nrgsQ []int32,
	x []int16,
	aQ12 *[2][silk.MaxLPCOrder]int16,
	gains []int32,
	subfrLength int32,
	lpcOrder int32,
) {
	const halfSubFr = silk.NBSubFr >> 1
	offset := lpcOrder + subfrLength

	var S [silk.MaxLPCOrder]int16
	var lpcRes [(silk.MaxFrameLength + silk.NBSubFr*silk.MaxLPCOrder) / 2]int16

	xOff := int32(0)
	for i := int32(0); i < 2; i++ {
		// Reset state, run analysis filter for this half-frame's coefs.
		for j := range S[:lpcOrder] {
			S[j] = 0
		}
		dsp.LPCAnalysisFilter(x[xOff:], aQ12[i][:], S[:], lpcRes[:],
			halfSubFr*offset, lpcOrder)

		// Skip preceding-sample warm-up, then measure each subframe.
		resOff := lpcOrder
		for j := int32(0); j < halfSubFr; j++ {
			nrg, rshift := dsp.SumSqrShift(lpcRes[resOff:], subfrLength)
			idx := i*halfSubFr + j
			nrgs[idx] = nrg
			nrgsQ[idx] = -rshift
			resOff += offset
		}
		xOff += halfSubFr * offset
	}

	// Apply squared gains: scale up the energy by gain² in matching Q.
	for i := int32(0); i < silk.NBSubFr; i++ {
		lz1 := fix.Clz32(nrgs[i]) - 1
		lz2 := fix.Clz32(gains[i]) - 1
		tmp := fix.LShift32(gains[i], lz2)
		tmp = fix.Smmul(tmp, tmp) // Q(2*lz2 - 32)

		nrgs[i] = fix.Smmul(tmp, fix.LShift32(nrgs[i], lz1))
		nrgsQ[i] += lz1 + 2*lz2 - 32 - 32
	}
}
