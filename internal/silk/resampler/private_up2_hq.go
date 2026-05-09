package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// PrivateUp2HQ — SKP_Silk_resampler_private_up2_HQ.
//
// 2x upsample using two cascaded all-pass sections per output phase plus a
// notch just above Nyquist. State[6]: indices 0-1 even-phase all-pass, 2-3
// odd-phase all-pass, 4-5 notch states.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_up2_HQ.c.
func PrivateUp2HQ(state []int32, out []int16, in []int16, length int32) {
	a0 := tables.Resampler_up2_hq_0
	a1 := tables.Resampler_up2_hq_1
	notch := tables.Resampler_up2_hq_notch

	for k := int32(0); k < length; k++ {
		in32 := fix.LShift32(int32(in[k]), 10)

		// Even output sample.
		Y := in32 - state[0]
		X := fix.SmulWB(Y, int32(a0[0]))
		out321 := state[0] + X
		state[0] = in32 + X

		Y = out321 - state[1]
		X = fix.SmlaWB(Y, Y, int32(a0[1]))
		out322 := state[1] + X
		state[1] = out321 + X

		out322 = fix.SmlaWB(out322, state[5], int32(notch[2]))
		out322 = fix.SmlaWB(out322, state[4], int32(notch[1]))
		out321 = fix.SmlaWB(out322, state[4], int32(notch[0]))
		state[5] = out322 - state[5]

		out[2*k] = int16(fix.Sat16(fix.RShift32(
			fix.SmlaWB(256, out321, int32(notch[3])), 9)))

		// Odd output sample.
		Y = in32 - state[2]
		X = fix.SmulWB(Y, int32(a1[0]))
		out321 = state[2] + X
		state[2] = in32 + X

		Y = out321 - state[3]
		X = fix.SmlaWB(Y, Y, int32(a1[1]))
		out322 = state[3] + X
		state[3] = out321 + X

		out322 = fix.SmlaWB(out322, state[4], int32(notch[2]))
		out322 = fix.SmlaWB(out322, state[5], int32(notch[1]))
		out321 = fix.SmlaWB(out322, state[5], int32(notch[0]))
		state[4] = out322 - state[4]

		out[2*k+1] = int16(fix.Sat16(fix.RShift32(
			fix.SmlaWB(256, out321, int32(notch[3])), 9)))
	}
}

// PrivateUp2HQWrapper — SKP_Silk_resampler_private_up2_HQ_wrapper.
// Routes to the underlying SIIR state slice in the State struct.
func PrivateUp2HQWrapper(s *State, out []int16, in []int16, length int32) {
	PrivateUp2HQ(s.SIIR[:], out, in, length)
}
