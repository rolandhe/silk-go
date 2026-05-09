package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

const resamplerOrderFIR144 = 6

// privateIIRFIRInterpol — INTERPOL kernel: 6-tap polyphase FIR using the
// 144-fraction RAM table. Operates on int16 buf samples.
func privateIIRFIRInterpol(out []int16, buf []int16,
	maxIndexQ16, indexIncrementQ16 int32) []int16 {

	for indexQ16 := int32(0); indexQ16 < maxIndexQ16; indexQ16 += indexIncrementQ16 {
		tableIdx := fix.SmulWB(indexQ16&0xFFFF, 144)
		bufOff := indexQ16 >> 16
		row := tables.Resampler_frac_FIR_144[tableIdx]
		mirror := tables.Resampler_frac_FIR_144[143-tableIdx]

		res := fix.SmulBB(int32(buf[bufOff+0]), int32(row[0]))
		res = fix.SmlaBB(res, int32(buf[bufOff+1]), int32(row[1]))
		res = fix.SmlaBB(res, int32(buf[bufOff+2]), int32(row[2]))
		res = fix.SmlaBB(res, int32(buf[bufOff+3]), int32(mirror[2]))
		res = fix.SmlaBB(res, int32(buf[bufOff+4]), int32(mirror[1]))
		res = fix.SmlaBB(res, int32(buf[bufOff+5]), int32(mirror[0]))

		out[0] = int16(fix.Sat16(fix.RShiftRound(res, 15)))
		out = out[1:]
	}
	return out
}

// PrivateIIRFIR — SKP_Silk_resampler_private_IIR_FIR.
//
// Default upsample path: optional 2x upsampler (chosen at Init time) or
// ARMA4 anti-image filter, then 6-tap polyphase FIR via the 144-fraction
// LUT. The FIR delay line is stored as 6 int16 in SFIRInt.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_IIR_FIR.c.
func PrivateIIRFIR(s *State, out []int16, in []int16, inLen int32) {
	const carry = resamplerOrderFIR144 // 6 int16 carried between calls
	var buf [2*ResamplerMaxBatchSizeIn + resamplerOrderFIR144]int16

	// Carry the previous 6 int16 into buf head.
	copy(buf[:carry], s.SFIRInt[:carry])

	indexIncrement := s.InvRatioQ16
	outOff := int32(0)

	var nSamplesIn int32
	for {
		nSamplesIn = inLen
		if nSamplesIn > s.BatchSize {
			nSamplesIn = s.BatchSize
		}

		writeLen := nSamplesIn << uint(s.Input2x)
		if s.Input2x == 1 {
			s.up2Function(s.SIIR[:], buf[carry:carry+writeLen], in[:nSamplesIn], nSamplesIn)
		} else {
			PrivateARMA4(s.SIIR[:4], buf[carry:carry+writeLen], in[:nSamplesIn], s.Coefs, nSamplesIn)
		}

		maxIndex := fix.LShift32(nSamplesIn, 16+s.Input2x)
		newOut := privateIIRFIRInterpol(out[outOff:], buf[:], maxIndex, indexIncrement)
		written := int32(len(out[outOff:]) - len(newOut))
		outOff += written

		in = in[nSamplesIn:]
		inLen -= nSamplesIn
		if inLen <= 0 {
			break
		}
		// Carry tail of buf to head: copy from [nSamplesIn<<input2x:] (length carry).
		shift := nSamplesIn << uint(s.Input2x)
		copy(buf[:carry], buf[shift:shift+carry])
	}

	// Save tail to state.
	tailStart := nSamplesIn << uint(s.Input2x)
	copy(s.SFIRInt[:carry], buf[tailStart:tailStart+carry])
}
