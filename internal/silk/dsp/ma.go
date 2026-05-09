package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// MAPrediction — SKP_Silk_MA_Prediction.
//
// Variable-order MA prediction-error filter. State is in Q12; B
// coefficients are Q12 int16; in/out are int16.
//
// Translated from vendor/silk/src/SKP_Silk_MA.c.
func MAPrediction(in []int16, B []int16, S []int32, out []int16, length int32, order int32) {
	for k := int32(0); k < length; k++ {
		in16 := int32(in[k])
		out32 := fix.LShift32(in16, 12) - S[0]
		out32 = fix.RShiftRound(out32, 12)

		for d := int32(0); d < order-1; d++ {
			S[d] = fix.SmlaBBOvflw(S[d+1], in16, int32(B[d]))
		}
		S[order-1] = fix.SmulBB(in16, int32(B[order-1]))

		out[k] = int16(fix.Sat16(out32))
	}
}

// LPCAnalysisFilter — SKP_Silk_LPC_analysis_filter.
//
// Variable-order LPC analysis filter (inverse of LPC synthesis). State [Order]
// in Q0; B are Q12 prediction coefs.
//
// Translated from vendor/silk/src/SKP_Silk_MA.c (the second function in the file).
func LPCAnalysisFilter(in []int16, B []int16, S []int16, out []int16, length int32, order int32) {
	orderHalf := order >> 1
	for k := int32(0); k < length; k++ {
		SA := S[0]
		var out32Q12 int32
		for j := int32(0); j < orderHalf-1; j++ {
			idx := 2*j + 1
			SB := S[idx]
			S[idx] = SA
			out32Q12 = fix.SmlaBB(out32Q12, int32(SA), int32(B[idx-1]))
			out32Q12 = fix.SmlaBB(out32Q12, int32(SB), int32(B[idx]))
			SA = S[idx+1]
			S[idx+1] = SB
		}
		// Epilog.
		SB := S[order-1]
		S[order-1] = SA
		out32Q12 = fix.SmlaBB(out32Q12, int32(SA), int32(B[order-2]))
		out32Q12 = fix.SmlaBB(out32Q12, int32(SB), int32(B[order-1]))

		out32Q12 = fix.SubSat32(fix.LShift32(int32(in[k]), 12), out32Q12)
		out32 := fix.RShiftRound(out32Q12, 12)
		out[k] = int16(fix.Sat16(out32))

		S[0] = in[k]
	}
}
