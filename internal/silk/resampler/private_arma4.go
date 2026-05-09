package resampler

import "github.com/rolandhe/silk-go/internal/silk/fix"

// PrivateARMA4 — SKP_Silk_resampler_private_ARMA4.
//
// 4th-order ARMA filter (two cascaded biquads). Coefficients are packed:
//
//	{ B1_Q14[1], B2_Q14[1], -A1_Q14[1], -A1_Q14[2], -A2_Q14[1], -A2_Q14[2], gain_Q16 }
//
// where B*_Q14[0], B*_Q14[2], A*_Q14[0] are all 16384 (=1.0 in Q14). State[4]
// holds the per-biquad delay-line accumulators in Q6.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_ARMA4.c.
func PrivateARMA4(state []int32, out []int16, in []int16, coef []int16, length int32) {
	for k := int32(0); k < length; k++ {
		inQ8 := fix.LShift32(int32(in[k]), 8)

		out1Q8 := fix.AddLShift32(inQ8, state[0], 2)
		out2Q8 := fix.AddLShift32(out1Q8, state[2], 2)

		X := fix.SmlaWB(state[1], inQ8, int32(coef[0]))
		state[0] = fix.SmlaWB(X, out1Q8, int32(coef[2]))

		X = fix.SmlaWB(state[3], out1Q8, int32(coef[1]))
		state[2] = fix.SmlaWB(X, out2Q8, int32(coef[4]))

		state[1] = fix.SmlaWB(fix.RShift32(inQ8, 2), out1Q8, int32(coef[3]))
		state[3] = fix.SmlaWB(fix.RShift32(out1Q8, 2), out2Q8, int32(coef[5]))

		out[k] = int16(fix.Sat16(fix.RShift32(
			fix.SmlaWB(128, out2Q8, int32(coef[6])), 8)))
	}
}
