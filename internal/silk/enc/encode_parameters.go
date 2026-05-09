package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dec"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// EncodeParameters — SKP_Silk_encode_parameters.
//
// Range-encode the per-frame side information into psRC. Order matches
// the decoder (see dec.DecodeParameters):
//   - sample rate (first frame in packet only)
//   - signal type + quantizer offset (joint after first frame)
//   - gains (independent first, delta-coded thereafter)
//   - NLSF MSVQ path + interpolation factor
//   - voiced extras: pitch lag/contour, LTP gains/scale
//   - RNG seed
//   - excitation pulses (delegated to dec.EncodePulses, which already
//     houses the encoder-side shell coder + sign coder)
//   - VAD flag
//
// `q` is the quantized excitation produced by NSQ/NSQDelDec.
//
// Translated from vendor/silk/src/SKP_Silk_encode_parameters.c.
func EncodeParameters(c *CommonState, ctrl *CommonControl, psRC *rangecoder.State, q []int8) {
	// Sample rate — first frame only.
	if c.NFramesInPayloadBuf == 0 {
		var ix int32
		for i := int32(0); i < 3; i++ {
			if tables.SamplingRates_table[i] == c.FsKHz {
				ix = i
				break
			}
		}
		// (the C source breaks out at the match but `i` is left at 3 for
		// 24 kHz — so the encoded index is whichever loop variable
		// survives, which for SWB is 3. We replicate that.)
		if c.FsKHz == 24 {
			ix = 3
		}
		psRC.Encode(ix, tables.SamplingRates_CDF[:])
	}

	// Sigtype + quantizer offset.
	typeOffset := 2*ctrl.Sigtype + ctrl.QuantOffsetType
	if c.NFramesInPayloadBuf == 0 {
		psRC.Encode(typeOffset, tables.Type_offset_CDF[:])
	} else {
		psRC.Encode(typeOffset, tables.Type_offset_joint_CDF[c.TypeOffsetPrev][:])
	}
	c.TypeOffsetPrev = typeOffset

	// Gains: first subframe is independent or delta-coded depending on
	// packet position; the rest are always delta.
	if c.NFramesInPayloadBuf == 0 {
		psRC.Encode(ctrl.GainsIndices[0], tables.Gain_CDF[ctrl.Sigtype][:])
	} else {
		psRC.Encode(ctrl.GainsIndices[0], tables.Delta_gain_CDF[:])
	}
	for i := int32(1); i < silk.NBSubFr; i++ {
		psRC.Encode(ctrl.GainsIndices[i], tables.Delta_gain_CDF[:])
	}

	// NLSF: walk the codebook stages.
	cb := c.NLSFCB[ctrl.Sigtype]
	indices := ctrl.NLSFIndices[:cb.NStages]
	psRC.EncodeMulti(indices, cb.StartPtr)

	// NLSF interpolation factor.
	psRC.Encode(ctrl.NLSFInterpCoefQ2, tables.NLSF_interpolation_factor_CDF[:])

	if ctrl.Sigtype == silk.SigTypeVoiced {
		// Pitch lag — choice of CDF depends on Fs.
		switch c.FsKHz {
		case 8:
			psRC.Encode(ctrl.LagIndex, tables.Pitch_lag_NB_CDF[:])
		case 12:
			psRC.Encode(ctrl.LagIndex, tables.Pitch_lag_MB_CDF[:])
		case 16:
			psRC.Encode(ctrl.LagIndex, tables.Pitch_lag_WB_CDF[:])
		default:
			psRC.Encode(ctrl.LagIndex, tables.Pitch_lag_SWB_CDF[:])
		}

		// Pitch contour — 8 kHz uses a smaller stage-2 codebook.
		if c.FsKHz == 8 {
			psRC.Encode(ctrl.ContourIndex, tables.Pitch_contour_NB_CDF[:])
		} else {
			psRC.Encode(ctrl.ContourIndex, tables.Pitch_contour_CDF[:])
		}

		// LTP gains: codebook id then per-subframe entry.
		psRC.Encode(ctrl.PERIndex, tables.LTP_per_index_CDF[:])
		for k := int32(0); k < silk.NBSubFr; k++ {
			psRC.Encode(ctrl.LTPIndex[k], tables.LTPGainCDFPtrs[ctrl.PERIndex])
		}

		// LTP scale.
		psRC.Encode(ctrl.LTPScaleIndex, tables.LTPscale_CDF[:])
	}

	// RNG seed.
	psRC.Encode(ctrl.Seed, tables.Seed_CDF[:])

	// Excitation pulses — encoder helper lives in dec for historical
	// round-trip-test reasons; works fine here.
	dec.EncodePulses(psRC, ctrl.Sigtype, ctrl.QuantOffsetType, q, c.FrameLength)

	// VAD flag.
	psRC.Encode(c.VadFlag, tables.Vadflag_CDF[:])
}
