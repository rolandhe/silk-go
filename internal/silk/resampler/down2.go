package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// Down2 — SKP_Silk_resampler_down2.
//
// Two-pole all-pass downsampler by 2 (mediocre quality). State[2] in/out.
// Internal arithmetic is in Q10.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_down2.c.
func Down2(state []int32, out []int16, in []int16, inLen int32) {
	len2 := inLen >> 1

	for k := int32(0); k < len2; k++ {
		// Even input sample → first all-pass section.
		in32 := fix.LShift32(int32(in[2*k]), 10)
		Y := in32 - state[0]
		X := fix.SmlaWB(Y, Y, int32(tables.Resampler_down2_1))
		out32 := state[0] + X
		state[0] = in32 + X

		// Odd input sample → second all-pass section, summed.
		in32 = fix.LShift32(int32(in[2*k+1]), 10)
		Y = in32 - state[1]
		X = fix.SmulWB(Y, int32(tables.Resampler_down2_0))
		out32 = out32 + state[1] + X
		state[1] = in32 + X

		out[k] = int16(fix.Sat16(fix.RShiftRound(out32, 11)))
	}
}
