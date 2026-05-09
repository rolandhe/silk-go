package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// maxOrderLPC mirrors SKP_Silk_MAX_ORDER_LPC from SKP_Silk_SigProc_FIX.h.
const maxOrderLPC = 16

// Schur — SKP_Silk_schur.
//
// Compute Schur reflection coefficients (Q15 int16) from the autocorrelation
// `c[order+1]`. Returns the residual energy. Faster than Schur64 but lower
// precision (uses SmlaWB everywhere).
//
// Translated from vendor/silk/src/SKP_Silk_schur.c.
func Schur(rcQ15 []int16, c []int32, order int32) int32 {
	var C [maxOrderLPC + 1][2]int32

	// Normalize lag-0 to Q30-like range.
	lz := fix.Clz32(c[0])
	switch {
	case lz < 2:
		// Shift right by 1.
		for k := int32(0); k <= order; k++ {
			C[k][0] = c[k] >> 1
			C[k][1] = c[k] >> 1
		}
	case lz > 2:
		shift := lz - 2
		for k := int32(0); k <= order; k++ {
			C[k][0] = fix.LShift32(c[k], shift)
			C[k][1] = fix.LShift32(c[k], shift)
		}
	default:
		for k := int32(0); k <= order; k++ {
			C[k][0] = c[k]
			C[k][1] = c[k]
		}
	}

	for k := int32(0); k < order; k++ {
		// Reflection coefficient.
		denom := C[0][1] >> 15
		if denom < 1 {
			denom = 1
		}
		rcTmp := -fix.Div32By16(C[k+1][0], denom)
		rcTmp = fix.Sat16(rcTmp)
		rcQ15[k] = int16(rcTmp)

		// Update correlations.
		for n := int32(0); n < order-k; n++ {
			Ctmp1 := C[n+k+1][0]
			Ctmp2 := C[n][1]
			C[n+k+1][0] = fix.SmlaWB(Ctmp1, fix.LShift32(Ctmp2, 1), rcTmp)
			C[n][1] = fix.SmlaWB(Ctmp2, fix.LShift32(Ctmp1, 1), rcTmp)
		}
	}
	return C[0][1]
}
