package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Biquad — SKP_Silk_biquad.
//
// Second-order ARMA filter. B (Q13) is the [3]-tap MA, A (Q13) the [2]-tap
// AR, S the [2] state.
//
// Translated from vendor/silk/src/SKP_Silk_biquad.c.
func Biquad(in []int16, B []int16, A []int16, S []int32, out []int16, length int32) {
	S0 := S[0]
	S1 := S[1]
	A0Neg := -int32(A[0])
	A1Neg := -int32(A[1])
	for k := int32(0); k < length; k++ {
		in16 := int32(in[k])
		out32 := fix.SmlaBB(S0, in16, int32(B[0]))

		S0 = fix.SmlaBB(S1, in16, int32(B[1]))
		S0 += fix.LShift32(fix.SmulWB(out32, A0Neg), 3)

		S1 = fix.LShift32(fix.SmulWB(out32, A1Neg), 3)
		S1 = fix.SmlaBB(S1, in16, int32(B[2]))
		tmp := fix.RShiftRound(out32, 13) + 1
		out[k] = int16(fix.Sat16(tmp))
	}
	S[0] = S0
	S[1] = S1
}

// BiquadAlt — SKP_Silk_biquad_alt.
//
// Higher-precision biquad: BQ28 (3 taps), AQ28 (2 taps), Q12 state.
// Direct Form II Transposed; assumes len is even (the C source asserts this).
//
// Translated from vendor/silk/src/SKP_Silk_biquad_alt.c.
func BiquadAlt(in []int16, BQ28 []int32, AQ28 []int32, S []int32, out []int16, length int32) {
	A0L := (-AQ28[0]) & 0x3FFF
	A0U := fix.RShift32(-AQ28[0], 14)
	A1L := (-AQ28[1]) & 0x3FFF
	A1U := fix.RShift32(-AQ28[1], 14)

	for k := int32(0); k < length; k++ {
		inval := int32(in[k])
		out32Q14 := fix.LShift32(fix.SmlaWB(S[0], BQ28[0], inval), 2)

		S[0] = S[1] + fix.RShiftRound(fix.SmulWB(out32Q14, A0L), 14)
		S[0] = fix.SmlaWB(S[0], out32Q14, A0U)
		S[0] = fix.SmlaWB(S[0], BQ28[1], inval)

		S[1] = fix.RShiftRound(fix.SmulWB(out32Q14, A1L), 14)
		S[1] = fix.SmlaWB(S[1], out32Q14, A1U)
		S[1] = fix.SmlaWB(S[1], BQ28[2], inval)

		out[k] = int16(fix.Sat16(fix.RShift32(out32Q14+(1<<14)-1, 14)))
	}
}
