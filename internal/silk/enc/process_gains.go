package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// ProcessGainsFIX — SKP_Silk_process_gains_FIX.
//
// Final gain pipeline for the current frame:
//  1. Voiced-only LTP-coding-gain attenuation.
//  2. Soft limit on per-subframe gain × residual energy so the quantized
//     signal can't exceed a target peak set by current_SNR_dB_Q7.
//  3. Quantize the gains via dsp.GainsQuant (encoder path).
//  4. Pick a quantizer offset type for voiced frames based on combined
//     LTP coding gain + spectral tilt.
//  5. Compute the rate-distortion Lambda from the user complexity, the
//     VAD speech activity, the input/coding quality, and the chosen
//     quantizer offset.
//
// Translated from vendor/silk/src/SKP_Silk_process_gains_FIX.c.
func ProcessGainsFIX(psEnc *StateFIX, ctrl *ControlFIX) {
	psShape := &psEnc.SShape

	// Step 1: gain reduction proportional to LTP coding gain (voiced only).
	if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		// s = -sigm( (LTPredCodGain - 12) / 4 )
		sQ16 := -dsp.SigmQ15(fix.RShiftRound(ctrl.LTPredCodGainQ7-fix.FixConst32(12.0, 7), 4))
		for k := 0; k < silk.NBSubFr; k++ {
			ctrl.GainsQ16[k] = fix.SmlaWB(ctrl.GainsQ16[k], ctrl.GainsQ16[k], sQ16)
		}
	}

	// Step 2: soft-limit gain² + residual_energy / max_peak² per subframe.
	// InvMaxSqrVal_Q16 = log2lin( (70_dB - SNR_dB) / 3 ) / subfr_length
	invMaxSqrValQ16 := dsp.Log2Lin(fix.SmulWB(
		fix.FixConst32(70.0, 7)-ctrl.CurrentSNRdBQ7,
		fix.FixConst32(0.33, 16))) / psEnc.Cmn.SubfrLength

	for k := 0; k < silk.NBSubFr; k++ {
		resNrg := ctrl.ResNrg[k]
		resPart := fix.SmulWW(resNrg, invMaxSqrValQ16)
		// Re-scale by ResNrgQ.
		switch q := ctrl.ResNrgQ[k]; {
		case q > 0:
			if q < 32 {
				resPart = fix.RShiftRound(resPart, q)
			} else {
				resPart = 0
			}
		case q < 0:
			// Left shift, with saturation guard against overflow.
			shift := -q
			if resPart > fix.RShift32(0x7FFFFFFF, shift) {
				resPart = 0x7FFFFFFF
			} else {
				resPart = fix.LShift32(resPart, shift)
			}
		}

		gain := ctrl.GainsQ16[k]
		gainSq := fix.AddSat32(resPart, fix.Smmul(gain, gain))
		if gainSq < 0x7FFF { // SKP_int16_MAX
			// Refine with higher precision (Q16 head room + SmlaWW).
			gainSq = fix.SmlaWW(fix.LShift32(resPart, 16), gain, gain)
			gain = fix.SqrtApprox(gainSq) // Q8
			ctrl.GainsQ16[k] = fix.LShiftSat32(gain, 8)
		} else {
			gain = fix.SqrtApprox(gainSq) // Q0
			ctrl.GainsQ16[k] = fix.LShiftSat32(gain, 16)
		}
	}

	// Step 3: gain quantization (writes indices + replaces GainsQ16 with
	// the quantized values).
	dsp.GainsQuant(ctrl.Cmn.GainsIndices[:], ctrl.GainsQ16[:],
		&psShape.LastGainIndex, psEnc.Cmn.NFramesInPayloadBuf)

	// Step 4: voiced quantizer-offset selector.
	if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		// Combined LTP gain + tilt: hi → small offset, lo → larger offset
		// (which spreads quantization noise across more pulses).
		if ctrl.LTPredCodGainQ7+fix.RShift32(ctrl.InputTiltQ15, 8) > fix.FixConst32(1.0, 7) {
			ctrl.Cmn.QuantOffsetType = 0
		} else {
			ctrl.Cmn.QuantOffsetType = 1
		}
	}

	// Step 5: rate-distortion Lambda.
	quantOffsetQ10 := int32(tables.Quantization_Offsets_Q10[ctrl.Cmn.Sigtype][ctrl.Cmn.QuantOffsetType])
	ctrl.LambdaQ10 = fix.FixConst32(float64(silk.LambdaOffset), 10) +
		fix.SmulBB(fix.FixConst32(float64(silk.LambdaDelayedDecisions), 10), psEnc.Cmn.NStatesDelayedDecision) +
		fix.SmulWB(fix.FixConst32(float64(silk.LambdaSpeechAct), 18), psEnc.SpeechActivityQ8) +
		fix.SmulWB(fix.FixConst32(float64(silk.LambdaInputQuality), 12), ctrl.InputQualityQ14) +
		fix.SmulWB(fix.FixConst32(float64(silk.LambdaCodingQuality), 12), ctrl.CodingQualityQ14) +
		fix.SmulWB(fix.FixConst32(float64(silk.LambdaQuantOffset), 16), quantOffsetQ10)
}
