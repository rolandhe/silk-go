package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// sigm LUTs — mirror SKP_Silk_sigm_Q15.c. The values come from
//
//	round(1024 * ([1/(1+exp(-(1:5))), 1] - 1/(1+exp(-(0:5)))))   (slopes Q10)
//	round(32767 * 1/(1+exp(-(0:5))))                              (positive Q15)
//	round(32767 * 1/(1+exp((0:5))))                               (negative Q15)
var (
	sigmLUTSlopeQ10 = [6]int32{237, 153, 73, 30, 12, 7}
	sigmLUTPosQ15   = [6]int32{16384, 23955, 28861, 31213, 32178, 32548}
	sigmLUTNegQ15   = [6]int32{16384, 8812, 3906, 1554, 589, 219}
)

// SigmQ15 — SKP_Silk_sigm_Q15.
//
// Approximate sigmoid: returns round(32767 / (1 + exp(-x))) where x = inQ5/32.
// Clipped to [0, 32767]. Lookup table with linear interpolation over each
// step of 1.0 in real domain (= 32 in Q5).
//
// Translated from vendor/silk/src/SKP_Silk_sigm_Q15.c.
func SigmQ15(inQ5 int32) int32 {
	if inQ5 < 0 {
		inQ5 = -inQ5
		if inQ5 >= 6*32 {
			return 0
		}
		ind := fix.RShift32(inQ5, 5)
		return sigmLUTNegQ15[ind] - fix.SmulBB(sigmLUTSlopeQ10[ind], inQ5&0x1F)
	}
	if inQ5 >= 6*32 {
		return 32767
	}
	ind := fix.RShift32(inQ5, 5)
	return sigmLUTPosQ15[ind] + fix.SmulBB(sigmLUTSlopeQ10[ind], inQ5&0x1F)
}
