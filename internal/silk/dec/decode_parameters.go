package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// DecodeParameters — SKP_Silk_decode_parameters.
//
// Read all per-frame side information from the range coder: sample rate
// (first frame only), signal type/quant offset, gains, NLSFs, pitch (if
// voiced), LTP coefficients/scale, RNG seed, excitation pulses, VAD
// flag, frame-termination indicator. Writes into psDec.SRC.Error on
// invalid sampling rate; writes Excitation into q[].
//
// Translated from vendor/silk/src/SKP_Silk_decode_parameters.c.
func DecodeParameters(psDec *State, ctrl *Control, q []int32, fullDecoding bool) {
	rc := &psDec.SRC

	// Decode sampling rate (first frame of packet only).
	if psDec.NFramesDecoded == 0 {
		Ix := rc.Decode(tables.SamplingRates_CDF[:], tables.SamplingRates_offset)
		if Ix < 0 || Ix > 3 {
			rc.Error = silk.RangeCoderIllegalSampling
			return
		}
		fsKHzDec := tables.SamplingRates_table[Ix]
		DecoderSetFs(psDec, fsKHzDec)
	}

	// Sigtype + QuantOffsetType (joint conditional after first frame).
	var Ix int32
	if psDec.NFramesDecoded == 0 {
		Ix = rc.Decode(tables.Type_offset_CDF[:], tables.Type_offset_CDF_offset)
	} else {
		Ix = rc.Decode(tables.Type_offset_joint_CDF[psDec.TypeOffsetPrev][:], tables.Type_offset_CDF_offset)
	}
	ctrl.Sigtype = fix.RShift32(Ix, 1)
	ctrl.QuantOffsetType = Ix & 1
	psDec.TypeOffsetPrev = Ix

	// Gains: first subframe is independent or delta-coded depending on packet
	// position; remaining subframes are always delta.
	var gainsIndices [silk.NBSubFr]int32
	if psDec.NFramesDecoded == 0 {
		gainsIndices[0] = rc.Decode(tables.Gain_CDF[ctrl.Sigtype][:], tables.Gain_CDF_offset)
	} else {
		gainsIndices[0] = rc.Decode(tables.Delta_gain_CDF[:], tables.Delta_gain_CDF_offset)
	}
	for i := int32(1); i < silk.NBSubFr; i++ {
		gainsIndices[i] = rc.Decode(tables.Delta_gain_CDF[:], tables.Delta_gain_CDF_offset)
	}
	GainsDequant(ctrl.GainsQ16[:], gainsIndices[:], &psDec.LastGainIndex, psDec.NFramesDecoded)

	// NLSF: pick voiced/unvoiced codebook, range-decode the path indices,
	// then MSVQ-decode the path.
	cb := psDec.NLSFCB[ctrl.Sigtype]
	nlsfIndices := rc.DecodeMulti(cb.StartPtr, cb.MiddleIx)
	pNLSFQ15 := make([]int32, psDec.LPCOrder)
	nlsf.MSVQDecode(pNLSFQ15, cb, nlsfIndices, psDec.LPCOrder)

	// NLSF interpolation factor.
	ctrl.NLSFInterpCoefQ2 = rc.Decode(
		tables.NLSF_interpolation_factor_CDF[:],
		tables.NLSF_interpolation_factor_offset)
	if psDec.FirstFrameAfterReset == 1 {
		ctrl.NLSFInterpCoefQ2 = 4
	}

	if fullDecoding {
		// AR coefficients for the second half-frame (always present).
		nlsf.NLSF2AStable(ctrl.PredCoefQ12[1][:], pNLSFQ15, psDec.LPCOrder)

		if ctrl.NLSFInterpCoefQ2 < 4 {
			// Interpolate against the previous frame's NLSF1.
			pNLSF0 := make([]int32, psDec.LPCOrder)
			for i := int32(0); i < psDec.LPCOrder; i++ {
				pNLSF0[i] = psDec.PrevNLSFQ15[i] + fix.RShift32(
					fix.Mul(ctrl.NLSFInterpCoefQ2, pNLSFQ15[i]-psDec.PrevNLSFQ15[i]), 2)
			}
			nlsf.NLSF2AStable(ctrl.PredCoefQ12[0][:], pNLSF0, psDec.LPCOrder)
		} else {
			copy(ctrl.PredCoefQ12[0][:psDec.LPCOrder], ctrl.PredCoefQ12[1][:psDec.LPCOrder])
		}
	}

	// Save NLSF for next frame's interpolation.
	for i := int32(0); i < psDec.LPCOrder; i++ {
		psDec.PrevNLSFQ15[i] = pNLSFQ15[i]
	}

	// Bandwidth-expand both LPC sets after a packet loss.
	if psDec.LossCnt != 0 {
		dsp.BwExpander(ctrl.PredCoefQ12[0][:psDec.LPCOrder], psDec.LPCOrder, silk.BWEAfterLossQ16)
		dsp.BwExpander(ctrl.PredCoefQ12[1][:psDec.LPCOrder], psDec.LPCOrder, silk.BWEAfterLossQ16)
	}

	if ctrl.Sigtype == silk.SigTypeVoiced {
		// Pitch lag index — choice of CDF depends on Fs.
		var lagIdx int32
		switch psDec.FsKHz {
		case 8:
			lagIdx = rc.Decode(tables.Pitch_lag_NB_CDF[:], tables.Pitch_lag_NB_CDF_offset)
		case 12:
			lagIdx = rc.Decode(tables.Pitch_lag_MB_CDF[:], tables.Pitch_lag_MB_CDF_offset)
		case 16:
			lagIdx = rc.Decode(tables.Pitch_lag_WB_CDF[:], tables.Pitch_lag_WB_CDF_offset)
		default:
			lagIdx = rc.Decode(tables.Pitch_lag_SWB_CDF[:], tables.Pitch_lag_SWB_CDF_offset)
		}

		// Pitch contour index — 8 kHz uses a smaller stage-2 codebook.
		var contourIdx int32
		if psDec.FsKHz == 8 {
			contourIdx = rc.Decode(tables.Pitch_contour_NB_CDF[:], tables.Pitch_contour_NB_CDF_offset)
		} else {
			contourIdx = rc.Decode(tables.Pitch_contour_CDF[:], tables.Pitch_contour_CDF_offset)
		}
		DecodePitch(lagIdx, contourIdx, ctrl.PitchL[:], psDec.FsKHz)

		// LTP gains: codebook index then per-subframe entry pick.
		ctrl.PERIndex = rc.Decode(tables.LTP_per_index_CDF[:], tables.LTP_per_index_CDF_offset)
		cbkPtr := tables.LTPVQPtrsQ14[ctrl.PERIndex]
		for k := int32(0); k < silk.NBSubFr; k++ {
			Ixk := rc.Decode(tables.LTPGainCDFPtrs[ctrl.PERIndex],
				tables.LTP_gain_CDF_offsets[ctrl.PERIndex])
			for i := int32(0); i < silk.LTPOrder; i++ {
				ctrl.LTPCoefQ14[k*silk.LTPOrder+i] = cbkPtr[Ixk*silk.LTPOrder+i]
			}
		}

		// LTP scale.
		Ix = rc.Decode(tables.LTPscale_CDF[:], tables.LTPscale_offset)
		ctrl.LTPScaleQ14 = int32(tables.LTPScales_table_Q14[Ix])
	} else {
		// Unvoiced: zero pitch and LTP.
		for i := range ctrl.PitchL {
			ctrl.PitchL[i] = 0
		}
		for i := range ctrl.LTPCoefQ14 {
			ctrl.LTPCoefQ14[i] = 0
		}
		ctrl.PERIndex = 0
		ctrl.LTPScaleQ14 = 0
	}

	// RNG seed for unvoiced excitation.
	ctrl.Seed = rc.Decode(tables.Seed_CDF[:], tables.Seed_offset)

	// Pulses.
	DecodePulses(rc, ctrl, q, psDec.FrameLength)

	// VAD flag and frame-termination indicator.
	psDec.VadFlag = rc.Decode(tables.Vadflag_CDF[:], tables.Vadflag_offset)
	psDec.FrameTermination = rc.Decode(tables.FrameTermination_CDF[:], tables.FrameTermination_offset)

	// Track remaining bytes; trigger end-of-stream check if exhausted.
	_, nBytesUsed := rc.GetLength()
	psDec.NBytesLeft = rc.BufferLength - nBytesUsed
	if psDec.NBytesLeft < 0 {
		rc.Error = silk.RangeCoderReadBeyondBuffer
	}
	if psDec.NBytesLeft == 0 {
		rc.CheckAfterDecoding()
	}
}
