package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// DecodePulses — SKP_Silk_decode_pulses.
//
// Read the rate level, then per-shell-block pulse counts, then the shell
// codes, LSB extension bits and signs to fully reconstruct one frame's
// excitation vector q[].
//
// Translated from vendor/silk/src/SKP_Silk_decode_pulses.c.
func DecodePulses(rc *rangecoder.State, ctrl *Control, q []int32, frameLength int32) {
	// Decode rate level.
	rateCDF := tables.Rate_levels_CDF[ctrl.Sigtype][:]
	ctrl.RateLevelIndex = rc.Decode(rateCDF, tables.Rate_levels_CDF_offset)

	iter := frameLength / silk.ShellCodecFrameLength

	// Sum-weighted-pulses decoding for each shell block.
	var sumPulses [silk.MaxNBShellBlocks]int32
	var nLshifts [silk.MaxNBShellBlocks]int32
	cdfPtr := tables.Pulses_per_block_CDF[ctrl.RateLevelIndex][:]
	for i := int32(0); i < iter; i++ {
		sumPulses[i] = rc.Decode(cdfPtr, tables.Pulses_per_block_CDF_offset)
		// LSB indication: keep reading until we see a count ≤ MaxPulses.
		for sumPulses[i] == silk.MaxPulses+1 {
			nLshifts[i]++
			sumPulses[i] = rc.Decode(
				tables.Pulses_per_block_CDF[silk.NRateLevels-1][:],
				tables.Pulses_per_block_CDF_offset)
		}
	}

	// Shell decoding per block.
	for i := int32(0); i < iter; i++ {
		off := i * silk.ShellCodecFrameLength
		if sumPulses[i] > 0 {
			ShellDecoder(q[off:off+silk.ShellCodecFrameLength], rc, sumPulses[i])
		} else {
			for k := int32(0); k < silk.ShellCodecFrameLength; k++ {
				q[off+k] = 0
			}
		}
	}

	// LSB decoding for blocks that had nLshifts > 0.
	for i := int32(0); i < iter; i++ {
		if nLshifts[i] > 0 {
			off := i * silk.ShellCodecFrameLength
			lsbCDF := tables.Lsb_CDF[:]
			for k := int32(0); k < silk.ShellCodecFrameLength; k++ {
				absQ := q[off+k]
				for j := int32(0); j < nLshifts[i]; j++ {
					absQ = fix.LShift32(absQ, 1)
					absQ += rc.Decode(lsbCDF, 1)
				}
				q[off+k] = absQ
			}
		}
	}

	// Sign decoding (re-attaches signs to the magnitudes).
	DecodeSigns(rc, q, frameLength, ctrl.Sigtype, ctrl.QuantOffsetType, ctrl.RateLevelIndex)
}
