package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/ltp"
)

// FindPredCoefsFIX — SKP_Silk_find_pred_coefs_FIX.
//
// Picks all per-frame prediction coefficients:
//  1. Per-subframe weights derived from the inverse minimum gain.
//  2. (Voiced) LTP analysis → coefficient quantization → scaling control,
//     then build the LTP residual into LPC_in_pre.
//  3. (Unvoiced) just gain-scale the input into LPC_in_pre and zero the
//     LTP coefficients.
//  4. Burg LPC on LPC_in_pre, optional NLSF interpolation search.
//  5. NLSF stabilization + MSVQ quantization (delegated to ProcessNLSFsFIX).
//  6. Quantized-LPC residual energy per subframe.
//
// resPitch is the residual signal produced earlier by pitch analysis (used
// only on the voiced path as the LTP target).
//
// Translated from vendor/silk/src/SKP_Silk_find_pred_coefs_FIX.c.
func FindPredCoefsFIX(psEnc *StateFIX, ctrl *ControlFIX, resPitch []int16) {
	var WLTP [silk.NBSubFr * silk.LTPOrder * silk.LTPOrder]int32
	var invGainsQ16 [silk.NBSubFr]int32
	var localGains [silk.NBSubFr]int32
	var wghtQ15 [silk.NBSubFr]int32
	var ltpCorrsRshift [silk.NBSubFr]int32
	var NLSFQ15 [silk.MaxLPCOrder]int32
	var lpcInPre [silk.NBSubFr*silk.MaxLPCOrder + silk.MaxFrameLength]int16

	// Step 1: per-subframe weights from inverse minimum gain.
	minGainQ16 := int32(0x7FFFFFFF) >> 6
	for i := int32(0); i < silk.NBSubFr; i++ {
		minGainQ16 = fix.MinInt(minGainQ16, ctrl.GainsQ16[i])
	}
	for i := int32(0); i < silk.NBSubFr; i++ {
		// invGains_Q16 = min_gain / gain[i] in Q14, then promoted to Q16.
		invGainsQ16[i] = fix.Div32VarQ(minGainQ16, ctrl.GainsQ16[i], 16-2)
		// Lower bound 363 — keeps Wght_Q15 ≥ 1 in the next line.
		invGainsQ16[i] = fix.MaxInt(invGainsQ16[i], 363)
		tmp := fix.SmulWB(invGainsQ16[i], invGainsQ16[i])
		wghtQ15[i] = fix.RShift32(tmp, 1)
		localGains[i] = (1 << 16) / invGainsQ16[i]
	}

	frameLen := psEnc.Cmn.FrameLength
	subfrLen := psEnc.Cmn.SubfrLength
	predOrder := psEnc.Cmn.PredictLPCOrder

	if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		// Step 2a: LTP analysis on the pitch residual, then coefficient
		// quantization + scale control + LTP residual filter.
		ltp.FindLTP(
			ctrl.LTPCoefQ14[:],
			WLTP[:],
			&ctrl.LTPredCodGainQ7,
			resPitch,
			resPitch[fix.RShift32(frameLen, 1):],
			ctrl.Cmn.PitchL[:],
			wghtQ15[:],
			subfrLen,
			frameLen,
			ltpCorrsRshift[:],
		)

		ltp.QuantGains(ctrl.LTPCoefQ14[:], ctrl.Cmn.LTPIndex[:], &ctrl.Cmn.PERIndex,
			WLTP[:], psEnc.MuLTPQ8, psEnc.Cmn.LTPQuantLowComplexity)

		// Wire transient state through to ScaleCtrl. The encoder-state
		// fields stay authoritative across calls.
		sc := ltp.ScaleCtrlState{
			LTPredCodGainQ7:     ctrl.LTPredCodGainQ7,
			HPLTPredCodGainQ7:   psEnc.HPLTPredCodGainQ7,
			PrevLTPredCodGainQ7: psEnc.PrevLTPredCodGainQ7,
			PacketLossPerc:      psEnc.Cmn.PacketLossPerc,
			NFramesInPayloadBuf: psEnc.Cmn.NFramesInPayloadBuf,
			PacketSizeMs:        psEnc.Cmn.PacketSizeMs,
		}
		ltp.ScaleCtrl(&sc)
		psEnc.HPLTPredCodGainQ7 = sc.HPLTPredCodGainQ7
		psEnc.PrevLTPredCodGainQ7 = sc.PrevLTPredCodGainQ7
		ctrl.Cmn.LTPScaleIndex = sc.LTPScaleIndex
		ctrl.LTPScaleQ14 = sc.LTPScaleQ14

		// LTP residual: x_buf[frame_length - LPC_order ..] is the start of
		// each subframe's preceding-history window.
		xStart := frameLen - predOrder
		ltp.AnalysisFilter(
			lpcInPre[:],
			psEnc.XBuf[:],
			xStart,
			ctrl.LTPCoefQ14[:],
			ctrl.Cmn.PitchL[:],
			invGainsQ16[:],
			subfrLen,
			predOrder,
		)
	} else {
		// Step 2b: unvoiced — just gain-scale the input, no LTP.
		xOff := frameLen - predOrder
		preStride := subfrLen + predOrder
		preOff := int32(0)
		for i := int32(0); i < silk.NBSubFr; i++ {
			dsp.ScaleCopyVector16(
				lpcInPre[preOff:preOff+preStride],
				psEnc.XBuf[xOff:xOff+preStride],
				invGainsQ16[i],
				preStride,
			)
			preOff += preStride
			xOff += subfrLen
		}
		for i := range ctrl.LTPCoefQ14 {
			ctrl.LTPCoefQ14[i] = 0
		}
		ctrl.LTPredCodGainQ7 = 0
	}

	// Steps 4–5: LPC on the LTP-filtered (or scaled) input + NLSF quantize.
	useInterp := psEnc.Cmn.UseInterpolatedNLSFs * (1 - psEnc.Cmn.FirstFrameAfterReset)
	FindLPCFIX(
		NLSFQ15[:],
		&ctrl.Cmn.NLSFInterpCoefQ2,
		psEnc.SPred.PrevNLSFqQ15[:],
		useInterp,
		predOrder,
		lpcInPre[:],
		subfrLen+predOrder,
	)

	ProcessNLSFsFIX(psEnc, ctrl, NLSFQ15[:])

	// Step 6: residual energy with quantized LPC + scaled gains.
	ResidualEnergyFIX(
		ctrl.ResNrg[:],
		ctrl.ResNrgQ[:],
		lpcInPre[:],
		&ctrl.PredCoefQ12,
		localGains[:],
		subfrLen,
		predOrder,
	)

	// Carry NLSF for the next frame's interpolation/fluctuation reduction.
	for i := int32(0); i < predOrder; i++ {
		psEnc.SPred.PrevNLSFqQ15[i] = NLSFQ15[i]
	}
}
