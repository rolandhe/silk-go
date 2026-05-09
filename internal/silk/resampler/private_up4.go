package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// PrivateUp4 — SKP_Silk_resampler_private_up4.
//
// 4x upsample (very low quality, only used >96 kHz). Each input sample
// duplicates 2x → 4x via two all-pass sections plus same-sample doubling.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_up4.c.
func PrivateUp4(state []int32, out []int16, in []int16, length int32) {
	for k := int32(0); k < length; k++ {
		in32 := fix.LShift32(int32(in[k]), 10)

		// Even output via first all-pass.
		Y := in32 - state[0]
		X := fix.SmulWB(Y, int32(tables.Resampler_up2_lq_0))
		out32 := state[0] + X
		state[0] = in32 + X
		v := int16(fix.Sat16(fix.RShiftRound(out32, 10)))
		out[4*k] = v
		out[4*k+1] = v

		// Odd output via second all-pass.
		Y = in32 - state[1]
		X = fix.SmlaWB(Y, Y, int32(tables.Resampler_up2_lq_1))
		out32 = state[1] + X
		state[1] = in32 + X
		v = int16(fix.Sat16(fix.RShiftRound(out32, 10)))
		out[4*k+2] = v
		out[4*k+3] = v
	}
}
