package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

const down23OrderFIR = 4

// ResamplerMaxBatchSizeIn — RESAMPLER_MAX_BATCH_SIZE_IN from
// SKP_Silk_resampler_private.h.
const ResamplerMaxBatchSizeIn = 480

// Down23 — SKP_Silk_resampler_down2_3.
//
// 2/3 downsampler: AR2 anti-alias + polyphase FIR pickoff. State[6] in/out
// (4 entries store the FIR delay line; the C source aliases sIIR onto S[4..5]).
//
// Translated from vendor/silk/src/SKP_Silk_resampler_down2_3.c.
func Down23(state []int32, out []int16, in []int16, inLen int32) {
	coefs := tables.Resampler_2_3_COEFS_LQ[:]
	var buf [ResamplerMaxBatchSizeIn + down23OrderFIR]int32

	// Copy buffered FIR delay-line state into the head of buf.
	copy(buf[:down23OrderFIR], state[:down23OrderFIR])

	outOff := int32(0)
	var nSamplesIn int32
	for {
		nSamplesIn = inLen
		if nSamplesIn > ResamplerMaxBatchSizeIn {
			nSamplesIn = ResamplerMaxBatchSizeIn
		}

		// AR2 over the next nSamplesIn input samples; output goes after the FIR delay line.
		PrivateAR2(state[down23OrderFIR:down23OrderFIR+2],
			buf[down23OrderFIR:down23OrderFIR+nSamplesIn],
			in, coefs[:2], nSamplesIn)

		// FIR interpolation: 2 outputs per 3 buffered input samples.
		bufOff := int32(0)
		counter := nSamplesIn
		for counter > 2 {
			res := fix.SmulWB(buf[bufOff+0], int32(coefs[2]))
			res = fix.SmlaWB(res, buf[bufOff+1], int32(coefs[3]))
			res = fix.SmlaWB(res, buf[bufOff+2], int32(coefs[5]))
			res = fix.SmlaWB(res, buf[bufOff+3], int32(coefs[4]))
			out[outOff] = int16(fix.Sat16(fix.RShiftRound(res, 6)))
			outOff++

			res = fix.SmulWB(buf[bufOff+1], int32(coefs[4]))
			res = fix.SmlaWB(res, buf[bufOff+2], int32(coefs[5]))
			res = fix.SmlaWB(res, buf[bufOff+3], int32(coefs[3]))
			res = fix.SmlaWB(res, buf[bufOff+4], int32(coefs[2]))
			out[outOff] = int16(fix.Sat16(fix.RShiftRound(res, 6)))
			outOff++

			bufOff += 3
			counter -= 3
		}

		in = in[nSamplesIn:]
		inLen -= nSamplesIn
		if inLen <= 0 {
			break
		}
		// Carry the tail of the filtered signal back to the start.
		copy(buf[:down23OrderFIR], buf[nSamplesIn:nSamplesIn+down23OrderFIR])
	}

	// Save tail to state for next call.
	copy(state[:down23OrderFIR], buf[nSamplesIn:nSamplesIn+down23OrderFIR])
}
