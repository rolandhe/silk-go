package resampler

import "github.com/rolandhe/silk-go/internal/silk/fix"

// PrivateAR2 — SKP_Silk_resampler_private_AR2.
//
// Second-order AR filter with single delay elements. State[2] in/out;
// outQ8 is the Q8-scaled output. AQ14 are the two AR coefficients in Q14.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_AR2.c.
func PrivateAR2(state []int32, outQ8 []int32, in []int16, AQ14 []int16, length int32) {
	for k := int32(0); k < length; k++ {
		out32 := fix.AddLShift32(state[0], int32(in[k]), 8)
		outQ8[k] = out32
		out32 = fix.LShift32(out32, 2)
		state[0] = fix.SmlaWB(state[1], out32, int32(AQ14[0]))
		state[1] = fix.SmulWB(out32, int32(AQ14[1]))
	}
}
