package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
)

// ProcessNLSFsFIX — SKP_Silk_process_NLSFs_FIX.
//
// Limit, stabilize, and quantize the unquantized NLSF vector for the
// current frame:
//  1. Pick rate-distortion mu values, gated by signal type and the VAD
//     speech-activity (and sparseness for unvoiced frames).
//  2. Compute Laroia weights for the input NLSF; if NLSF interpolation is
//     enabled, blend in the weights for the interpolated first half.
//  3. Run the MSVQ encoder against the appropriate codebook.
//  4. Convert quantized NLSFs to LPC coefficients (second half).
//  5. Either also do the interpolated first half, or copy the second
//     half's LPCs into the first.
//
// The pNLSFQ15 buffer is used both as input (unquantized NLSFs) and output
// (quantized NLSFs replace them in place).
//
// Translated from vendor/silk/src/SKP_Silk_process_NLSFs_FIX.c.
func ProcessNLSFsFIX(psEnc *StateFIX, ctrl *ControlFIX, pNLSFQ15 []int32) {
	var pNLSFWQ6 [silk.MaxLPCOrder]int32
	var pNLSF0TempQ15 [silk.MaxLPCOrder]int32
	var pNLSFW0TempQ6 [silk.MaxLPCOrder]int32

	// Step 1: mu values for the rate-distortion search.
	var nlsfMuQ15, nlsfMuFlucRedQ16 int32
	if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		// mu       ≈ 0.002 - 0.001 * activity
		// mu_fluc  ≈ 0.1   - 0.05  * activity
		nlsfMuQ15 = fix.SmlaWB(66, -8388, psEnc.SpeechActivityQ8)
		nlsfMuFlucRedQ16 = fix.SmlaWB(6554, -838848, psEnc.SpeechActivityQ8)
	} else {
		// mu       ≈ 0.005 - 0.004 * activity
		// mu_fluc  ≈ 0.2   - 0.1   * activity - 0.1 * sparseness
		nlsfMuQ15 = fix.SmlaWB(164, -33554, psEnc.SpeechActivityQ8)
		nlsfMuFlucRedQ16 = fix.SmlaWB(13107, -1677696, psEnc.SpeechActivityQ8+ctrl.SparsenessQ8)
	}
	if nlsfMuQ15 < 1 {
		nlsfMuQ15 = 1
	}

	// Step 2: NLSF Laroia weights.
	nlsf.VQWeightsLaroia(pNLSFWQ6[:], pNLSFQ15, psEnc.Cmn.PredictLPCOrder)

	doInterpolate := psEnc.Cmn.UseInterpolatedNLSFs == 1 && ctrl.Cmn.NLSFInterpCoefQ2 < (1<<2)
	if doInterpolate {
		// Interpolated first-half NLSF + its own Laroia weights.
		dsp.Interpolate(pNLSF0TempQ15[:], psEnc.SPred.PrevNLSFqQ15[:], pNLSFQ15,
			ctrl.Cmn.NLSFInterpCoefQ2, psEnc.Cmn.PredictLPCOrder)
		nlsf.VQWeightsLaroia(pNLSFW0TempQ6[:], pNLSF0TempQ15[:], psEnc.Cmn.PredictLPCOrder)

		// Blend: weights = ½·second-half + (interp²/16)·first-half.
		iSqrQ15 := fix.LShift32(fix.SmulBB(ctrl.Cmn.NLSFInterpCoefQ2, ctrl.Cmn.NLSFInterpCoefQ2), 11)
		for i := int32(0); i < psEnc.Cmn.PredictLPCOrder; i++ {
			pNLSFWQ6[i] = fix.SmlaWB(fix.RShift32(pNLSFWQ6[i], 1), pNLSFW0TempQ6[i], iSqrQ15)
		}
	}

	// Step 3: MSVQ quantization.
	cb := psEnc.Cmn.NLSFCB[ctrl.Cmn.Sigtype]
	nlsf.MSVQEncode(
		ctrl.Cmn.NLSFIndices[:],
		pNLSFQ15,
		cb,
		psEnc.SPred.PrevNLSFqQ15[:],
		pNLSFWQ6[:],
		nlsfMuQ15,
		nlsfMuFlucRedQ16,
		psEnc.Cmn.NLSFMSVQSurvivors,
		psEnc.Cmn.PredictLPCOrder,
		psEnc.Cmn.FirstFrameAfterReset,
	)

	// Step 4: NLSF → LPC for the second half.
	nlsf.NLSF2AStable(ctrl.PredCoefQ12[1][:], pNLSFQ15, psEnc.Cmn.PredictLPCOrder)

	// Step 5: first half — either interpolate-then-NLSF2A, or copy.
	if doInterpolate {
		dsp.Interpolate(pNLSF0TempQ15[:], psEnc.SPred.PrevNLSFqQ15[:], pNLSFQ15,
			ctrl.Cmn.NLSFInterpCoefQ2, psEnc.Cmn.PredictLPCOrder)
		nlsf.NLSF2AStable(ctrl.PredCoefQ12[0][:], pNLSF0TempQ15[:], psEnc.Cmn.PredictLPCOrder)
	} else {
		copy(ctrl.PredCoefQ12[0][:psEnc.Cmn.PredictLPCOrder], ctrl.PredCoefQ12[1][:psEnc.Cmn.PredictLPCOrder])
	}
}
