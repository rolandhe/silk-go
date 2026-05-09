package nlsf

import "github.com/rolandhe/silk-go/internal/silk/fix"

// VQRateDistortion — SKP_Silk_NLSF_VQ_rate_distortion_FIX.
//
// Compute rate-distortion costs for N input vectors against one MSVQ
// codebook stage. Internally calls VQSumError to get weighted distortions,
// then adds μ·rate per row.
//
// Output rdQ20 has length stage.NVectors * N (row-major).
//
// Translated from vendor/silk/src/SKP_Silk_NLSF_VQ_rate_distortion_FIX.c.
func VQRateDistortion(rdQ20 []int32, stage *CBStage, inQ15 []int32, wQ6 []int32,
	rateAccQ5 []int32, muQ15 int32, N, lpcOrder int32) {

	VQSumError(rdQ20, inQ15, wQ6, stage.CBNLSFQ15, N, stage.NVectors, lpcOrder)

	for n := int32(0); n < N; n++ {
		off := n * stage.NVectors
		for i := int32(0); i < stage.NVectors; i++ {
			rate := rateAccQ5[n] + int32(stage.RatesQ5[i])
			rdQ20[off+i] = fix.SmlaBB(rdQ20[off+i], rate, muQ15)
		}
	}
}
