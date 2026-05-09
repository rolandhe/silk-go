package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// WarpedLPCAnalysisFilterFIX — SKP_Silk_warped_LPC_analysis_filter_FIX.
//
// Variable-warping LPC analysis filter built from cascaded all-pass
// sections. The warping factor lambda_Q16 is the same one used by
// noise_shape_analysis when WarpingQ16 > 0. State must be `order + 1`
// long; order must be even.
//
// Translated from vendor/silk/src/SKP_Silk_prefilter_FIX.c.
func WarpedLPCAnalysisFilterFIX(state []int32, res []int16, coefQ13 []int16, input []int16,
	lambdaQ16 int16, length, order int32) {

	for n := int32(0); n < length; n++ {
		// Lowpass section.
		tmp2 := fix.SmlaWB(state[0], state[1], int32(lambdaQ16))
		state[0] = fix.LShift32(int32(input[n]), 14)
		// First all-pass.
		tmp1 := fix.SmlaWB(state[1], state[2]-tmp2, int32(lambdaQ16))
		state[1] = tmp2
		accQ11 := fix.SmulWB(tmp2, int32(coefQ13[0]))

		for i := int32(2); i < order; i += 2 {
			tmp2 = fix.SmlaWB(state[i], state[i+1]-tmp1, int32(lambdaQ16))
			state[i] = tmp1
			accQ11 = fix.SmlaWB(accQ11, tmp1, int32(coefQ13[i-1]))

			tmp1 = fix.SmlaWB(state[i+1], state[i+2]-tmp2, int32(lambdaQ16))
			state[i+1] = tmp2
			accQ11 = fix.SmlaWB(accQ11, tmp2, int32(coefQ13[i]))
		}
		state[order] = tmp1
		accQ11 = fix.SmlaWB(accQ11, tmp1, int32(coefQ13[order-1]))
		res[n] = int16(fix.Sat16(int32(input[n]) - fix.RShiftRound(accQ11, 11)))
	}
}

// prefiltSubFix — SKP_Silk_prefilt_FIX (the inner per-subframe shaper).
//
// Combines harmonic LTP shaping (3-tap FIR over the LTP shape buffer),
// spectral tilt, and LF-shaping coefficients into the per-sample
// prefilter output. State carries an LTP shape ring buffer plus two
// recursive AR/MA states.
func prefiltSubFix(P *PrefilterStateFIX, stResQ12 []int32, xw []int16,
	harmShapeFIRPackedQ12 int32, tiltQ14 int32, LFShpQ14 int32, lag, length int32) {

	LTPShpBuf := P.SLTPShp[:]
	LTPShpBufIdx := P.SLTPShpBufIdx
	sLFARShpQ12 := P.SLFARShpQ12
	sLFMAShpQ12 := P.SLFMAShpQ12

	for i := int32(0); i < length; i++ {
		var nLTPQ12 int32
		if lag > 0 {
			idx := lag + LTPShpBufIdx
			nLTPQ12 = fix.SmulBB(int32(LTPShpBuf[(idx-silk.HarmShapeFIRTaps/2-1)&silk.LTPMask]), harmShapeFIRPackedQ12)
			nLTPQ12 = fix.SmlaBT(nLTPQ12, int32(LTPShpBuf[(idx-silk.HarmShapeFIRTaps/2)&silk.LTPMask]), harmShapeFIRPackedQ12)
			nLTPQ12 = fix.SmlaBB(nLTPQ12, int32(LTPShpBuf[(idx-silk.HarmShapeFIRTaps/2+1)&silk.LTPMask]), harmShapeFIRPackedQ12)
		}

		nTiltQ10 := fix.SmulWB(sLFARShpQ12, tiltQ14)
		nLFQ10 := fix.SmlaWB(fix.SmulWT(sLFARShpQ12, LFShpQ14), sLFMAShpQ12, LFShpQ14)

		sLFARShpQ12 = stResQ12[i] - fix.LShift32(nTiltQ10, 2)
		sLFMAShpQ12 = sLFARShpQ12 - fix.LShift32(nLFQ10, 2)

		LTPShpBufIdx = (LTPShpBufIdx - 1) & silk.LTPMask
		LTPShpBuf[LTPShpBufIdx] = int16(fix.Sat16(fix.RShiftRound(sLFMAShpQ12, 12)))

		xw[i] = int16(fix.Sat16(fix.RShiftRound(sLFMAShpQ12-nLTPQ12, 12)))
	}

	P.SLFARShpQ12 = sLFARShpQ12
	P.SLFMAShpQ12 = sLFMAShpQ12
	P.SLTPShpBufIdx = LTPShpBufIdx
}

// PrefilterFIX — SKP_Silk_prefilter_FIX.
//
// Per-subframe prefilter that produces the NSQ-input weighted signal.
// Steps per subframe:
//  1. Lock the harmonic shaping coef pair (HarmShapeGain reduced by
//     HarmBoost) and pack it into a single int32 (two int16 halves) so
//     the inner loop can use SmlaBT.
//  2. Run the warped LPC analysis filter to get the short-term residual.
//  3. Apply a 2-tap FIR with packed B_Q12 (gainsPre + tilt-modified
//     coefficient) — the C source pun-loads this as int32 too.
//  4. Hand off to prefiltSubFix for harmonic + tilt + LF shaping.
//
// Translated from vendor/silk/src/SKP_Silk_prefilter_FIX.c.
func PrefilterFIX(psEnc *StateFIX, ctrl *ControlFIX, xw []int16, x []int16) {
	P := &psEnc.SPrefilt

	subfrLen := psEnc.Cmn.SubfrLength
	pxOff := int32(0)
	pxwOff := int32(0)
	lag := P.LagPrev

	var xFiltQ12 [silk.MaxFrameLength / silk.NBSubFr]int32
	var stRes [(silk.MaxFrameLength / silk.NBSubFr) + silk.MaxShapeLPCOrder]int16

	for k := int32(0); k < silk.NBSubFr; k++ {
		if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
			lag = ctrl.Cmn.PitchL[k]
		}

		// Pack the 3-tap harmonic shape FIR coefs into one int32.
		harmShapeGainQ12 := fix.SmulWB(ctrl.HarmShapeGainQ14[k], 16384-ctrl.HarmBoostQ14[k])
		harmShapeFIRPackedQ12 := fix.RShift32(harmShapeGainQ12, 2) |
			fix.LShift32(fix.RShift32(harmShapeGainQ12, 1), 16)
		tiltQ14 := ctrl.TiltQ14[k]
		LFShpQ14 := ctrl.LFShpQ14[k]
		AR1ShpQ13 := ctrl.AR1Q13[k*silk.MaxShapeLPCOrder : (k+1)*silk.MaxShapeLPCOrder]

		// Step 2: short-term FIR via warped LPC.
		WarpedLPCAnalysisFilterFIX(P.SARShp[:], stRes[:], AR1ShpQ13, x[pxOff:],
			int16(psEnc.Cmn.WarpingQ16), subfrLen, psEnc.Cmn.ShapingLPCOrder)

		// Step 3: 2-tap FIR with packed B_Q12 (low = gainsPre, high =
		// tilt-modified).
		BQ12Low := fix.RShiftRound(ctrl.GainsPreQ14[k], 2)
		tmp32 := fix.SmlaBB(fix.FixConst32(float64(silk.InputTilt), 26),
			ctrl.HarmBoostQ14[k], harmShapeGainQ12) // Q26
		tmp32 = fix.SmlaBB(tmp32, ctrl.CodingQualityQ14, fix.FixConst32(float64(silk.HighRateInputTilt), 12))
		tmp32 = fix.SmulWB(tmp32, -ctrl.GainsPreQ14[k])
		tmp32 = fix.RShiftRound(tmp32, 12)
		BQ12Hi := fix.Sat16(tmp32)
		BQ12 := BQ12Low | fix.LShift32(BQ12Hi, 16)

		xFiltQ12[0] = fix.SmlaBT(fix.SmulBB(int32(stRes[0]), BQ12), P.SHarmHP, BQ12)
		for j := int32(1); j < subfrLen; j++ {
			xFiltQ12[j] = fix.SmlaBT(fix.SmulBB(int32(stRes[j]), BQ12),
				int32(stRes[j-1]), BQ12)
		}
		P.SHarmHP = int32(stRes[subfrLen-1])

		prefiltSubFix(P, xFiltQ12[:], xw[pxwOff:],
			harmShapeFIRPackedQ12, tiltQ14, LFShpQ14, lag, subfrLen)

		pxOff += subfrLen
		pxwOff += subfrLen
	}

	P.LagPrev = ctrl.Cmn.PitchL[silk.NBSubFr-1]
}
