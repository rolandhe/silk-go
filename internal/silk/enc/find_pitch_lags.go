package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/lpc"
	"github.com/rolandhe/silk-go/internal/silk/pitch"
)

// FindPitchLagsFIX — SKP_Silk_find_pitch_lags_FIX.
//
//  1. Apply a sine window to the look-ahead-padded x_buf, compute the
//     autocorrelation + reflection coefficients via Schur, expand to a Q12
//     LPC, and run the analysis filter to get the LPC residual.
//  2. Build a pitch-correlation threshold scaled by the LPC order, the VAD
//     activity, the previous frame's voicing, and the input tilt.
//  3. Hand the residual to pitch.AnalysisCore, which writes pitchL[],
//     lagIndex, contourIndex, LTPCorr_Q15, and returns the voicing flag.
//
// The C source treats `x` as the current-frame pointer and computes
// `x_buf = x - frame_length`. We pass the explicit XBuf and an offset to
// the current frame so we can index backwards safely.
//
// `res` receives the LPC residual; the encoder reuses it as the pitch
// analysis input in the same call (and as the LTP target later).
//
// Translated from vendor/silk/src/SKP_Silk_find_pitch_lags_FIX.c.
func FindPitchLagsFIX(psEnc *StateFIX, ctrl *ControlFIX, res []int16, xBuf []int16, xCurrentOff int32) {
	psPredSt := &psEnc.SPred

	frameLen := psEnc.Cmn.FrameLength
	laPitch := psEnc.Cmn.LAPitch
	winLen := psPredSt.PitchLPCWinLength
	bufLen := laPitch + fix.LShift32(frameLen, 1)
	pitchLPCOrder := psEnc.Cmn.PitchEstimationLPCOrder

	// x_buf base = xCurrentOff - frame_length.
	xBufBase := xCurrentOff - frameLen

	// Step 1a: build a windowed signal in Wsig.
	var Wsig [silk.FindPitchLPCWinMax]int16
	winSrcOff := xBufBase + bufLen - winLen
	dstOff := int32(0)

	dsp.ApplySineWindow(Wsig[dstOff:], xBuf[winSrcOff:], 1, laPitch)
	dstOff += laPitch
	winSrcOff += laPitch

	midLen := winLen - fix.LShift32(laPitch, 1)
	copy(Wsig[dstOff:dstOff+midLen], xBuf[winSrcOff:winSrcOff+midLen])
	dstOff += midLen
	winSrcOff += midLen

	dsp.ApplySineWindow(Wsig[dstOff:], xBuf[winSrcOff:], 2, laPitch)

	// Step 1b: autocorrelation + Schur + LPC.
	var autoCorr [silk.MaxFindPitchLPCOrder + 1]int32
	var scale int32
	dsp.Autocorr(autoCorr[:], &scale, Wsig[:], winLen, pitchLPCOrder+1)
	autoCorr[0] = fix.SmlaWB(autoCorr[0], autoCorr[0],
		fix.FixConst32(float64(silk.FindPitchWhiteNoiseFraction), 16))

	var rcQ15 [silk.MaxFindPitchLPCOrder]int16
	resNrg := dsp.Schur(rcQ15[:], autoCorr[:], pitchLPCOrder)
	ctrl.PredGainQ16 = fix.Div32VarQ(autoCorr[0], fix.MaxInt(resNrg, 1), 16)

	var aQ24 [silk.MaxFindPitchLPCOrder]int32
	lpc.K2a(aQ24[:], rcQ15[:], pitchLPCOrder)

	var aQ12 [silk.MaxFindPitchLPCOrder]int16
	for i := int32(0); i < pitchLPCOrder; i++ {
		aQ12[i] = int16(fix.Sat16(fix.RShift32(aQ24[i], 12)))
	}
	dsp.BwExpander(aQ12[:], pitchLPCOrder,
		fix.FixConst32(float64(silk.FindPitchBandwithExpansion), 16))

	// Step 1c: LPC residual into res.
	var filtState [silk.MaxFindPitchLPCOrder]int32
	dsp.MAPrediction(xBuf[xBufBase:], aQ12[:], filtState[:], res, bufLen, pitchLPCOrder)
	for i := int32(0); i < pitchLPCOrder; i++ {
		res[i] = 0
	}

	// Step 2: pitch-correlation threshold.
	thrhldQ15 := fix.FixConst32(0.45, 15)
	thrhldQ15 = fix.SmlaBB(thrhldQ15, fix.FixConst32(-0.004, 15), pitchLPCOrder)
	thrhldQ15 = fix.SmlaBB(thrhldQ15, fix.FixConst32(-0.1, 7), psEnc.SpeechActivityQ8)
	thrhldQ15 = fix.SmlaBB(thrhldQ15, fix.FixConst32(0.15, 15), psEnc.Cmn.PrevSigtype)
	thrhldQ15 = fix.SmlaWB(thrhldQ15, fix.FixConst32(-0.1, 16), ctrl.InputTiltQ15)
	thrhldQ15 = fix.Sat16(thrhldQ15)

	// Step 3: dispatch to the multi-stage pitch core.
	ctrl.Cmn.Sigtype = pitch.AnalysisCore(
		res,
		ctrl.Cmn.PitchL[:],
		&ctrl.Cmn.LagIndex,
		&ctrl.Cmn.ContourIndex,
		&psEnc.LTPCorrQ15,
		psEnc.Cmn.PrevLag,
		psEnc.Cmn.PitchEstimationThresholdQ16,
		thrhldQ15,
		psEnc.Cmn.FsKHz,
		psEnc.Cmn.PitchEstimationComplexity,
		0, /* forLJC */
	)
}
