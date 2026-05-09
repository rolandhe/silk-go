package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Warped autocorrelation Q domains — mirror SKP_Silk_warped_autocorrelation_FIX.c.
const (
	warpedQC = 10
	warpedQS = 14
)

// WarpedAutocorrelation — SKP_Silk_warped_autocorrelation_FIX.
//
// Compute the autocorrelation of `input` after passing it through `order`/2
// first-order all-pass sections (frequency-warped). Output `corr[order+1]`
// is in Q(scale) where `scale = -(QC + lsh)` with lsh chosen to fit corr_QC
// into int32. Order must be even.
//
// Translated from vendor/silk/src/SKP_Silk_warped_autocorrelation_FIX.c.
func WarpedAutocorrelation(corr []int32, scale *int32, input []int16,
	warpingQ16 int16, length int32, order int32) {

	// Order must be even — keep as a runtime invariant; release builds in C
	// just SKP_assert it.
	var stateQS [maxOrderLPC + 1]int32
	var corrQC [maxOrderLPC + 1]int64

	for n := int32(0); n < length; n++ {
		tmp1QS := fix.LShift32(int32(input[n]), warpedQS)
		for i := int32(0); i < order; i += 2 {
			tmp2QS := fix.SmlaWB(stateQS[i], stateQS[i+1]-tmp1QS, int32(warpingQ16))
			stateQS[i] = tmp1QS
			corrQC[i] += fix.RShift64(fix.Smull(tmp1QS, stateQS[0]), 2*warpedQS-warpedQC)

			tmp1QS = fix.SmlaWB(stateQS[i+1], stateQS[i+2]-tmp2QS, int32(warpingQ16))
			stateQS[i+1] = tmp2QS
			corrQC[i+1] += fix.RShift64(fix.Smull(tmp2QS, stateQS[0]), 2*warpedQS-warpedQC)
		}
		stateQS[order] = tmp1QS
		corrQC[order] += fix.RShift64(fix.Smull(tmp1QS, stateQS[0]), 2*warpedQS-warpedQC)
	}

	lsh := fix.Clz64(corrQC[0]) - 35
	lsh = fix.Limit(lsh, -12-warpedQC, 30-warpedQC)
	*scale = -(warpedQC + lsh)

	if lsh >= 0 {
		for i := int32(0); i < order+1; i++ {
			corr[i] = int32(fix.LShift64(corrQC[i], lsh))
		}
	} else {
		for i := int32(0); i < order+1; i++ {
			corr[i] = int32(fix.RShift64(corrQC[i], -lsh))
		}
	}
}
