package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Schur64 — SKP_Silk_schur64.
//
// High-precision Schur reflection coefficient extraction. Output `rcQ16` is
// in Q16 (vs Schur's Q15). Uses Div32VarQ + Smmul instead of SmlaWB.
//
// Translated from vendor/silk/src/SKP_Silk_schur64.c.
func Schur64(rcQ16 []int32, c []int32, order int32) int32 {
	if c[0] <= 0 {
		for i := int32(0); i < order; i++ {
			rcQ16[i] = 0
		}
		return 0
	}

	var C [maxOrderLPC + 1][2]int32
	for k := int32(0); k <= order; k++ {
		C[k][0] = c[k]
		C[k][1] = c[k]
	}

	for k := int32(0); k < order; k++ {
		// Reflection coefficient: divide two Q30 values, get Q31 result.
		rcTmpQ31 := fix.Div32VarQ(-C[k+1][0], C[0][1], 31)
		rcQ16[k] = fix.RShiftRound(rcTmpQ31, 15)

		// Update correlations.
		for n := int32(0); n < order-k; n++ {
			Ctmp1 := C[n+k+1][0]
			Ctmp2 := C[n][1]
			C[n+k+1][0] = Ctmp1 + fix.Smmul(fix.LShift32(Ctmp2, 1), rcTmpQ31)
			C[n][1] = Ctmp2 + fix.Smmul(fix.LShift32(Ctmp1, 1), rcTmpQ31)
		}
	}
	return C[0][1]
}
