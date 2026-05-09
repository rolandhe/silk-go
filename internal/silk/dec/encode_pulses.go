package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// combineAndCheck — combine_and_check helper from encode_pulses.c.
//
// Pairs adjacent samples and checks against maxPulses; returns 1 if any
// pair sum exceeds maxPulses (signalling a need to scale down).
func combineAndCheck(comb, in []int32, maxPulses int32, length int32) int32 {
	for k := int32(0); k < length; k++ {
		sum := in[2*k] + in[2*k+1]
		if sum > maxPulses {
			return 1
		}
		comb[k] = sum
	}
	return 0
}

// EncodePulses — SKP_Silk_encode_pulses.
//
// Encoder counterpart of DecodePulses, ported here so the decoder's
// round-trip tests don't need a separate encoder package. This will move
// to the encoder (stage F) once that lands.
//
// Translated from vendor/silk/src/SKP_Silk_encode_pulses.c.
func EncodePulses(rc *rangecoder.State, sigtype, quantOffsetType int32, q []int8, frameLength int32) {
	iter := frameLength / silk.ShellCodecFrameLength

	// |q|. Buffer is at most MAX_FRAME_LENGTH long.
	absPulses := make([]int32, frameLength)
	for i := int32(0); i < frameLength; i++ {
		v := int32(q[i])
		if v < 0 {
			v = -v
		}
		absPulses[i] = v
	}

	// For each shell block: scale down until pair-sums fit in max_pulses_table.
	var sumPulses [silk.MaxNBShellBlocks]int32
	var nRshifts [silk.MaxNBShellBlocks]int32
	mptab := tables.Max_pulses_table[:]
	var pulsesComb [8]int32

	for i := int32(0); i < iter; i++ {
		ptr := absPulses[i*silk.ShellCodecFrameLength:]
		for {
			scaleDown := combineAndCheck(pulsesComb[:], ptr, mptab[0], 8)
			scaleDown += combineAndCheck(pulsesComb[:], pulsesComb[:], mptab[1], 4)
			scaleDown += combineAndCheck(pulsesComb[:], pulsesComb[:], mptab[2], 2)
			sumPulses[i] = pulsesComb[0] + pulsesComb[1]
			if sumPulses[i] > mptab[3] {
				scaleDown++
			}
			if scaleDown == 0 {
				break
			}
			nRshifts[i]++
			for k := int32(0); k < silk.ShellCodecFrameLength; k++ {
				ptr[k] = fix.RShift32(ptr[k], 1)
			}
		}
	}

	// Pick the rate level with the lowest total bit count for this frame.
	rateLevelIndex := int32(0)
	minSumBits := int32(0x7FFFFFFF)
	for k := int32(0); k < silk.NRateLevels-1; k++ {
		nBitsRow := tables.Pulses_per_block_BITS_Q6[k][:]
		sumBits := int32(tables.Rate_levels_BITS_Q6[sigtype][k])
		for i := int32(0); i < iter; i++ {
			if nRshifts[i] > 0 {
				sumBits += int32(nBitsRow[silk.MaxPulses+1])
			} else {
				sumBits += int32(nBitsRow[sumPulses[i]])
			}
		}
		if sumBits < minSumBits {
			minSumBits = sumBits
			rateLevelIndex = k
		}
	}
	rc.Encode(rateLevelIndex, tables.Rate_levels_CDF[sigtype][:])

	// Sum-weighted-pulses encoding.
	cdfRow := tables.Pulses_per_block_CDF[rateLevelIndex][:]
	fbCDF := tables.Pulses_per_block_CDF[silk.NRateLevels-1][:]
	for i := int32(0); i < iter; i++ {
		if nRshifts[i] == 0 {
			rc.Encode(sumPulses[i], cdfRow)
		} else {
			rc.Encode(silk.MaxPulses+1, cdfRow)
			for k := int32(0); k < nRshifts[i]-1; k++ {
				rc.Encode(silk.MaxPulses+1, fbCDF)
			}
			rc.Encode(sumPulses[i], fbCDF)
		}
	}

	// Shell encoding.
	for i := int32(0); i < iter; i++ {
		if sumPulses[i] > 0 {
			off := i * silk.ShellCodecFrameLength
			ShellEncoder(rc, absPulses[off:off+silk.ShellCodecFrameLength])
		}
	}

	// LSB encoding.
	lsbCDF := tables.Lsb_CDF[:]
	for i := int32(0); i < iter; i++ {
		if nRshifts[i] > 0 {
			pulsesPtr := q[i*silk.ShellCodecFrameLength:]
			nLS := nRshifts[i] - 1
			for k := int32(0); k < silk.ShellCodecFrameLength; k++ {
				// C: abs_q = (SKP_int8)SKP_abs(pulses_ptr[k]). The int8
				// cast wraps -128 → -128 (its own absolute value
				// doesn't fit in int8); re-cast so the LSB extraction
				// matches bit-for-bit on that single edge case.
				v := int32(pulsesPtr[k])
				if v < 0 {
					v = -v
				}
				absQ := int32(int8(v))
				for j := nLS; j > 0; j-- {
					bit := fix.RShift32(absQ, j) & 1
					rc.Encode(bit, lsbCDF)
				}
				rc.Encode(absQ&1, lsbCDF)
			}
		}
	}

	// Sign encoding.
	EncodeSigns(rc, q, frameLength, sigtype, quantOffsetType, rateLevelIndex)
}
