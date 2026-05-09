package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// Up2 — SKP_Silk_resampler_up2.
//
// Two-pole all-pass upsampler by 2 (low quality). State[2] in/out.
// Each input sample produces 2 output samples (even/odd), with each
// sample passed through a different all-pass section (Q10 internal).
//
// Translated from vendor/silk/src/SKP_Silk_resampler_up2.c.
func Up2(state []int32, out []int16, in []int16, length int32) {
	for k := int32(0); k < length; k++ {
		in32 := fix.LShift32(int32(in[k]), 10)

		// Even output sample.
		Y := in32 - state[0]
		X := fix.SmulWB(Y, int32(tables.Resampler_up2_lq_0))
		out32 := state[0] + X
		state[0] = in32 + X
		out[2*k] = int16(fix.Sat16(fix.RShiftRound(out32, 10)))

		// Odd output sample.
		Y = in32 - state[1]
		X = fix.SmlaWB(Y, Y, int32(tables.Resampler_up2_lq_1))
		out32 = state[1] + X
		state[1] = in32 + X
		out[2*k+1] = int16(fix.Sat16(fix.RShiftRound(out32, 10)))
	}
}
