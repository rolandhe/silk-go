package resampler

import "github.com/rolandhe/silk-go/internal/silk/fix"

const resamplerDownOrderFIR = 12

// privateDownFIRInterpol0 — INTERPOL0 path: FIR_Fracs == 1, 6 taps with
// symmetric coefficients (12 buffered samples, paired sum).
func privateDownFIRInterpol0(out []int16, buf2 []int32, firCoefs []int16,
	maxIndexQ16, indexIncrementQ16 int32) []int16 {
	for indexQ16 := int32(0); indexQ16 < maxIndexQ16; indexQ16 += indexIncrementQ16 {
		bufOff := indexQ16 >> 16
		res := fix.SmulWB(buf2[bufOff+0]+buf2[bufOff+11], int32(firCoefs[0]))
		res = fix.SmlaWB(res, buf2[bufOff+1]+buf2[bufOff+10], int32(firCoefs[1]))
		res = fix.SmlaWB(res, buf2[bufOff+2]+buf2[bufOff+9], int32(firCoefs[2]))
		res = fix.SmlaWB(res, buf2[bufOff+3]+buf2[bufOff+8], int32(firCoefs[3]))
		res = fix.SmlaWB(res, buf2[bufOff+4]+buf2[bufOff+7], int32(firCoefs[4]))
		res = fix.SmlaWB(res, buf2[bufOff+5]+buf2[bufOff+6], int32(firCoefs[5]))
		out[0] = int16(fix.Sat16(fix.RShiftRound(res, 6)))
		out = out[1:]
	}
	return out
}

// privateDownFIRInterpol1 — INTERPOL1 path: FIR_Fracs > 1, polyphase-with-fraction.
func privateDownFIRInterpol1(out []int16, buf2 []int32, firCoefs []int16,
	maxIndexQ16, indexIncrementQ16, firFracs int32) []int16 {
	for indexQ16 := int32(0); indexQ16 < maxIndexQ16; indexQ16 += indexIncrementQ16 {
		bufOff := indexQ16 >> 16
		interpolInd := fix.SmulWB(indexQ16&0xFFFF, firFracs)
		const half = resamplerDownOrderFIR / 2

		coefAOff := half * interpolInd
		res := fix.SmulWB(buf2[bufOff+0], int32(firCoefs[coefAOff+0]))
		res = fix.SmlaWB(res, buf2[bufOff+1], int32(firCoefs[coefAOff+1]))
		res = fix.SmlaWB(res, buf2[bufOff+2], int32(firCoefs[coefAOff+2]))
		res = fix.SmlaWB(res, buf2[bufOff+3], int32(firCoefs[coefAOff+3]))
		res = fix.SmlaWB(res, buf2[bufOff+4], int32(firCoefs[coefAOff+4]))
		res = fix.SmlaWB(res, buf2[bufOff+5], int32(firCoefs[coefAOff+5]))

		coefBOff := half * (firFracs - 1 - interpolInd)
		res = fix.SmlaWB(res, buf2[bufOff+11], int32(firCoefs[coefBOff+0]))
		res = fix.SmlaWB(res, buf2[bufOff+10], int32(firCoefs[coefBOff+1]))
		res = fix.SmlaWB(res, buf2[bufOff+9], int32(firCoefs[coefBOff+2]))
		res = fix.SmlaWB(res, buf2[bufOff+8], int32(firCoefs[coefBOff+3]))
		res = fix.SmlaWB(res, buf2[bufOff+7], int32(firCoefs[coefBOff+4]))
		res = fix.SmlaWB(res, buf2[bufOff+6], int32(firCoefs[coefBOff+5]))

		out[0] = int16(fix.Sat16(fix.RShiftRound(res, 6)))
		out = out[1:]
	}
	return out
}

// PrivateDownFIR — SKP_Silk_resampler_private_down_FIR.
//
// Hybrid 2x downsampler (optional) + 2nd-order AR + polyphase FIR. The
// state's sFIR slot stores the 12-sample int32 FIR delay line (Q8 from
// AR2 output).
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_down_FIR.c.
func PrivateDownFIR(s *State, out []int16, in []int16, inLen int32) {
	var buf1 [ResamplerMaxBatchSizeIn / 2]int16
	var buf2 [ResamplerMaxBatchSizeIn + resamplerDownOrderFIR]int32

	// Copy buffered samples to start of buf2.
	copy(buf2[:resamplerDownOrderFIR], s.SFIR[:resamplerDownOrderFIR])

	firCoefs := s.Coefs[2:]
	indexIncrement := s.InvRatioQ16
	outOff := int32(0)

	var nSamplesIn int32
	for {
		nSamplesIn = inLen
		if nSamplesIn > s.BatchSize {
			nSamplesIn = s.BatchSize
		}

		if s.Input2x == 1 {
			Down2(s.SDown2[:], buf1[:nSamplesIn>>1], in[:nSamplesIn], nSamplesIn)
			nSamplesIn >>= 1
			PrivateAR2(s.SIIR[:2], buf2[resamplerDownOrderFIR:resamplerDownOrderFIR+nSamplesIn],
				buf1[:nSamplesIn], s.Coefs[:2], nSamplesIn)
		} else {
			PrivateAR2(s.SIIR[:2], buf2[resamplerDownOrderFIR:resamplerDownOrderFIR+nSamplesIn],
				in[:nSamplesIn], s.Coefs[:2], nSamplesIn)
		}

		maxIndex := fix.LShift32(nSamplesIn, 16)
		var newOut []int16
		if s.FIRFracs == 1 {
			newOut = privateDownFIRInterpol0(out[outOff:], buf2[:], firCoefs, maxIndex, indexIncrement)
		} else {
			newOut = privateDownFIRInterpol1(out[outOff:], buf2[:], firCoefs, maxIndex, indexIncrement, s.FIRFracs)
		}
		written := int32(len(out[outOff:]) - len(newOut))
		outOff += written

		// Advance input pointer by nSamplesIn << input2x. Note:
		// we shadow `in` and decrement `inLen` separately.
		consumed := nSamplesIn << uint(s.Input2x)
		in = in[consumed:]
		inLen -= consumed

		if inLen <= s.Input2x {
			break
		}
		copy(buf2[:resamplerDownOrderFIR], buf2[nSamplesIn:nSamplesIn+resamplerDownOrderFIR])
	}

	copy(s.SFIR[:resamplerDownOrderFIR], buf2[nSamplesIn:nSamplesIn+resamplerDownOrderFIR])
}
