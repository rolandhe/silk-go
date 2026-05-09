package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// PrivateDown4 — SKP_Silk_resampler_private_down4.
//
// 4x downsample (very low quality, only used >96 kHz inputs). Pairs of
// adjacent samples are averaged before passing through the down2 all-pass
// pair. State[2] in/out, Q10 internal.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_down4.c.
func PrivateDown4(state []int32, out []int16, in []int16, inLen int32) {
	len4 := inLen >> 2
	for k := int32(0); k < len4; k++ {
		// Pair-sum first half, even all-pass.
		in32 := fix.LShift32(int32(in[4*k])+int32(in[4*k+1]), 9)
		Y := in32 - state[0]
		X := fix.SmlaWB(Y, Y, int32(tables.Resampler_down2_1))
		out32 := state[0] + X
		state[0] = in32 + X

		// Pair-sum second half, odd all-pass.
		in32 = fix.LShift32(int32(in[4*k+2])+int32(in[4*k+3]), 9)
		Y = in32 - state[1]
		X = fix.SmulWB(Y, int32(tables.Resampler_down2_0))
		out32 = out32 + state[1] + X
		state[1] = in32 + X

		out[k] = int16(fix.Sat16(fix.RShiftRound(out32, 11)))
	}
}
