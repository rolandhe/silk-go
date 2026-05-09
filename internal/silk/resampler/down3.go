package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

const down3OrderFIR = 6

// Down3 — SKP_Silk_resampler_down3.
//
// 1/3 downsampler: AR2 + symmetric 6-tap FIR. State[8]: indices [0..5] FIR
// delay, [6..7] AR2 state.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_down3.c.
func Down3(state []int32, out []int16, in []int16, inLen int32) {
	coefs := tables.Resampler_1_3_COEFS_LQ[:]
	var buf [ResamplerMaxBatchSizeIn + down3OrderFIR]int32

	copy(buf[:down3OrderFIR], state[:down3OrderFIR])

	outOff := int32(0)
	var nSamplesIn int32
	for {
		nSamplesIn = inLen
		if nSamplesIn > ResamplerMaxBatchSizeIn {
			nSamplesIn = ResamplerMaxBatchSizeIn
		}
		PrivateAR2(state[down3OrderFIR:down3OrderFIR+2],
			buf[down3OrderFIR:down3OrderFIR+nSamplesIn],
			in, coefs[:2], nSamplesIn)

		bufOff := int32(0)
		counter := nSamplesIn
		for counter > 2 {
			res := fix.SmulWB(buf[bufOff+0]+buf[bufOff+5], int32(coefs[2]))
			res = fix.SmlaWB(res, buf[bufOff+1]+buf[bufOff+4], int32(coefs[3]))
			res = fix.SmlaWB(res, buf[bufOff+2]+buf[bufOff+3], int32(coefs[4]))
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
		copy(buf[:down3OrderFIR], buf[nSamplesIn:nSamplesIn+down3OrderFIR])
	}
	copy(state[:down3OrderFIR], buf[nSamplesIn:nSamplesIn+down3OrderFIR])
}
