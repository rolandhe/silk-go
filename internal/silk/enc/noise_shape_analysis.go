package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/lpc"
)

// warpedGain — gain (Q16) needed to make warped filter coefficients have
// zero-mean log frequency response on the unwarped scale, so the result
// can be implemented as a minimum-phase monic filter.
//
// Translated from `warped_gain` in SKP_Silk_noise_shape_analysis_FIX.c.
func warpedGain(coefsQ24 []int32, lambdaQ16, order int32) int32 {
	lambda := -lambdaQ16
	gainQ24 := coefsQ24[order-1]
	for i := order - 2; i >= 0; i-- {
		gainQ24 = fix.SmlaWB(coefsQ24[i], gainQ24, lambda)
	}
	gainQ24 = fix.SmlaWB(fix.FixConst32(1.0, 24), gainQ24, -lambda)
	return fix.Inverse32VarQ(gainQ24, 40)
}

// limitWarpedCoefs — convert warped coefs to monic pseudo-warped coefs
// and bound their amplitude via iterative bandwidth expansion. The
// outer `iter` cap of 10 mirrors the C source — if we exhaust it the
// coefs were pathological; the C source has an `assert(0)` here, we
// silently return.
//
// Translated from `limit_warped_coefs` in SKP_Silk_noise_shape_analysis_FIX.c.
func limitWarpedCoefs(coefsSynQ24, coefsAnaQ24 []int32, lambdaQ16, limitQ24, order int32) {
	lambda := -lambdaQ16
	for i := order - 1; i > 0; i-- {
		coefsSynQ24[i-1] = fix.SmlaWB(coefsSynQ24[i-1], coefsSynQ24[i], lambda)
		coefsAnaQ24[i-1] = fix.SmlaWB(coefsAnaQ24[i-1], coefsAnaQ24[i], lambda)
	}
	lambda = -lambda
	nomQ16 := fix.SmlaWB(fix.FixConst32(1.0, 16), -lambda, lambda)
	denQ24 := fix.SmlaWB(fix.FixConst32(1.0, 24), coefsSynQ24[0], lambda)
	gainSynQ16 := fix.Div32VarQ(nomQ16, denQ24, 24)
	denQ24 = fix.SmlaWB(fix.FixConst32(1.0, 24), coefsAnaQ24[0], lambda)
	gainAnaQ16 := fix.Div32VarQ(nomQ16, denQ24, 24)
	for i := int32(0); i < order; i++ {
		coefsSynQ24[i] = fix.SmulWW(gainSynQ16, coefsSynQ24[i])
		coefsAnaQ24[i] = fix.SmulWW(gainAnaQ16, coefsAnaQ24[i])
	}

	for iter := int32(0); iter < 10; iter++ {
		// Find max absolute coefficient.
		maxabsQ24 := int32(-1)
		ind := int32(0)
		for i := int32(0); i < order; i++ {
			tmp := fix.MaxInt(fix.Abs32(coefsSynQ24[i]), fix.Abs32(coefsAnaQ24[i]))
			if tmp > maxabsQ24 {
				maxabsQ24 = tmp
				ind = i
			}
		}
		if maxabsQ24 <= limitQ24 {
			return
		}

		// Convert back to true warped coefficients.
		for i := int32(1); i < order; i++ {
			coefsSynQ24[i-1] = fix.SmlaWB(coefsSynQ24[i-1], coefsSynQ24[i], lambda)
			coefsAnaQ24[i-1] = fix.SmlaWB(coefsAnaQ24[i-1], coefsAnaQ24[i], lambda)
		}
		gainSynQ16 = fix.Inverse32VarQ(gainSynQ16, 32)
		gainAnaQ16 = fix.Inverse32VarQ(gainAnaQ16, 32)
		for i := int32(0); i < order; i++ {
			coefsSynQ24[i] = fix.SmulWW(gainSynQ16, coefsSynQ24[i])
			coefsAnaQ24[i] = fix.SmulWW(gainAnaQ16, coefsAnaQ24[i])
		}

		// Apply bandwidth expansion proportional to overshoot.
		chirpQ16 := fix.FixConst32(0.99, 16) - fix.Div32VarQ(
			fix.SmulWB(maxabsQ24-limitQ24, fix.SmlaBB(fix.FixConst32(0.8, 10), fix.FixConst32(0.1, 10), iter)),
			maxabsQ24*(ind+1), 22)
		dsp.BwExpander32(coefsSynQ24, order, chirpQ16)
		dsp.BwExpander32(coefsAnaQ24, order, chirpQ16)

		// Convert back to monic warped coefficients.
		lambda = -lambda
		for i := order - 1; i > 0; i-- {
			coefsSynQ24[i-1] = fix.SmlaWB(coefsSynQ24[i-1], coefsSynQ24[i], lambda)
			coefsAnaQ24[i-1] = fix.SmlaWB(coefsAnaQ24[i-1], coefsAnaQ24[i], lambda)
		}
		lambda = -lambda
		nomQ16 = fix.SmlaWB(fix.FixConst32(1.0, 16), -lambda, lambda)
		denQ24 = fix.SmlaWB(fix.FixConst32(1.0, 24), coefsSynQ24[0], lambda)
		gainSynQ16 = fix.Div32VarQ(nomQ16, denQ24, 24)
		denQ24 = fix.SmlaWB(fix.FixConst32(1.0, 24), coefsAnaQ24[0], lambda)
		gainAnaQ16 = fix.Div32VarQ(nomQ16, denQ24, 24)
		for i := int32(0); i < order; i++ {
			coefsSynQ24[i] = fix.SmulWW(gainSynQ16, coefsSynQ24[i])
			coefsAnaQ24[i] = fix.SmulWW(gainAnaQ16, coefsAnaQ24[i])
		}
	}
}

// NoiseShapeAnalysisFIX — SKP_Silk_noise_shape_analysis_FIX.
//
// The full noise-shaping configuration pass that runs after pitch +
// pre-prediction analysis and before NSQ:
//   - Pick a target SNR from BufferedInChannelMs and FEC overhead.
//   - Derive coding/input quality factors and a sparseness measure (used
//     as the unvoiced quantizer-offset signal).
//   - Compute per-subframe analysis (AR1) and synthesis (AR2) shaping
//     coefficients via Schur on a windowed signal — warped or plain
//     autocorrelation depending on `warping_Q16`.
//   - Set per-subframe Gains_Q16 (sqrt of residual energy), apply gain
//     tweaks for SNR, noise floor, fricative de-essing.
//   - Build the LF shaping coef pair and per-subframe Tilt + HarmBoost +
//     HarmShapeGain (smoothed across subframes via SShape state).
//
// `pitchRes` is the LPC residual produced by find_pitch_lags. `xBuf` is
// the encoder's input buffer; `xCurrentOff` is the offset of the current
// frame's first sample inside it — the C source treats `x` as a pointer
// and uses `x_ptr = x - la_shape`, so callers must guarantee at least
// `la_shape` preceding samples in xBuf.
//
// Translated from vendor/silk/src/SKP_Silk_noise_shape_analysis_FIX.c.
func NoiseShapeAnalysisFIX(psEnc *StateFIX, ctrl *ControlFIX, pitchRes, xBuf []int16, xCurrentOff int32) {
	psShapeSt := &psEnc.SShape

	// SNR control.
	ctrl.CurrentSNRdBQ7 = psEnc.SNRdBQ7 - fix.SmulWB(
		fix.LShift32(psEnc.BufferedInChannelMs, 7), fix.FixConst32(0.05, 16))
	if psEnc.SpeechActivityQ8 > fix.FixConst32(float64(silk.LBRRSpeechActivityThres), 8) {
		ctrl.CurrentSNRdBQ7 -= fix.RShift32(psEnc.InBandFECSNRCompQ8, 1)
	}

	// Quality factors.
	ctrl.InputQualityQ14 = fix.RShift32(ctrl.InputQualityBandsQ15[0]+ctrl.InputQualityBandsQ15[1], 2)
	ctrl.CodingQualityQ14 = fix.RShift32(
		dsp.SigmQ15(fix.RShiftRound(ctrl.CurrentSNRdBQ7-fix.FixConst32(18.0, 7), 4)), 1)

	// SNR adjustment.
	bQ8 := fix.FixConst32(1.0, 8) - psEnc.SpeechActivityQ8
	bQ8 = fix.SmulWB(fix.LShift32(bQ8, 8), bQ8)
	SNRAdjDBQ7 := fix.SmlaWB(ctrl.CurrentSNRdBQ7,
		fix.SmulBB(-fix.FixConst32(float64(silk.BGSNRDecrDB), 7)>>(4+1), bQ8),
		fix.SmulWB(fix.FixConst32(1.0, 14)+ctrl.InputQualityQ14, ctrl.CodingQualityQ14))

	if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		SNRAdjDBQ7 = fix.SmlaWB(SNRAdjDBQ7,
			fix.FixConst32(float64(silk.HarmSNRIncrDB), 8), psEnc.LTPCorrQ15)
	} else {
		SNRAdjDBQ7 = fix.SmlaWB(SNRAdjDBQ7,
			fix.SmlaWB(fix.FixConst32(6.0, 9), -fix.FixConst32(0.4, 18), ctrl.CurrentSNRdBQ7),
			fix.FixConst32(1.0, 14)-ctrl.InputQualityQ14)
	}

	// Sparseness — only affects unvoiced.
	if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		ctrl.Cmn.QuantOffsetType = 0
		ctrl.SparsenessQ8 = 0
	} else {
		nSamples := fix.LShift32(psEnc.Cmn.FsKHz, 1)
		var energyVariationQ7, logEnergyPrevQ7 int32
		off := int32(0)
		for k := int32(0); k < silk.FrameLengthMs/2; k++ {
			nrg, scale := dsp.SumSqrShift(pitchRes[off:], nSamples)
			nrg += fix.RShift32(nSamples, scale)
			logEnergyQ7 := dsp.Lin2Log(nrg)
			if k > 0 {
				diff := logEnergyQ7 - logEnergyPrevQ7
				if diff < 0 {
					diff = -diff
				}
				energyVariationQ7 += diff
			}
			logEnergyPrevQ7 = logEnergyQ7
			off += nSamples
		}
		ctrl.SparsenessQ8 = fix.RShift32(
			dsp.SigmQ15(fix.SmulWB(energyVariationQ7-fix.FixConst32(5.0, 7), fix.FixConst32(0.1, 16))), 7)

		if ctrl.SparsenessQ8 > fix.FixConst32(float64(silk.SparsenessThresholdQntOffset), 8) {
			ctrl.Cmn.QuantOffsetType = 0
		} else {
			ctrl.Cmn.QuantOffsetType = 1
		}
		SNRAdjDBQ7 = fix.SmlaWB(SNRAdjDBQ7,
			fix.FixConst32(float64(silk.SparseSNRIncrDB), 15),
			ctrl.SparsenessQ8-fix.FixConst32(0.5, 8))
	}

	// Bandwidth expansion controls.
	strengthQ16 := fix.SmulWB(ctrl.PredGainQ16,
		fix.FixConst32(float64(silk.FindPitchWhiteNoiseFraction), 16))
	BWExp1Q16 := fix.Div32VarQ(fix.FixConst32(float64(silk.BandwidthExpansion), 16),
		fix.SmlaWW(fix.FixConst32(1.0, 16), strengthQ16, strengthQ16), 16)
	BWExp2Q16 := BWExp1Q16
	deltaQ16 := fix.SmulWB(fix.FixConst32(1.0, 16)-fix.SmulBB(3, ctrl.CodingQualityQ14),
		fix.FixConst32(float64(silk.LowRateBandwidthExpansionDelta), 16))
	BWExp1Q16 -= deltaQ16
	BWExp2Q16 += deltaQ16
	// BWExp1 is applied after BWExp2 → make it relative.
	BWExp1Q16 = fix.LShift32(BWExp1Q16, 14) / fix.RShift32(BWExp2Q16, 2)

	var warpingQ16 int32
	if psEnc.Cmn.WarpingQ16 > 0 {
		warpingQ16 = fix.SmlaWB(psEnc.Cmn.WarpingQ16, ctrl.CodingQualityQ14, fix.FixConst32(0.01, 18))
	}

	// Per-subframe noise shaping.
	var autoCorr [silk.MaxShapeLPCOrder + 1]int32
	var reflCoefQ16 [silk.MaxShapeLPCOrder]int32
	var AR1Q24 [silk.MaxShapeLPCOrder]int32
	var AR2Q24 [silk.MaxShapeLPCOrder]int32
	var xWindowed [silk.ShapeLPCWinMax]int16

	xPtrOff := xCurrentOff - psEnc.Cmn.LAShape
	for k := int32(0); k < silk.NBSubFr; k++ {
		flatPart := psEnc.Cmn.FsKHz * 5
		slopePart := fix.RShift32(psEnc.Cmn.ShapeWinLength-flatPart, 1)

		// Sine ramp + flat + cosine ramp window.
		dsp.ApplySineWindow(xWindowed[:], xBuf[xPtrOff:], 1, slopePart)
		shift := slopePart
		copy(xWindowed[shift:shift+flatPart], xBuf[xPtrOff+shift:xPtrOff+shift+flatPart])
		shift += flatPart
		dsp.ApplySineWindow(xWindowed[shift:], xBuf[xPtrOff+shift:], 2, slopePart)

		xPtrOff += psEnc.Cmn.SubfrLength

		var scale int32
		if psEnc.Cmn.WarpingQ16 > 0 {
			dsp.WarpedAutocorrelation(autoCorr[:], &scale, xWindowed[:],
				int16(warpingQ16), psEnc.Cmn.ShapeWinLength, psEnc.Cmn.ShapingLPCOrder)
		} else {
			dsp.Autocorr(autoCorr[:], &scale, xWindowed[:],
				psEnc.Cmn.ShapeWinLength, psEnc.Cmn.ShapingLPCOrder+1)
		}

		// White-noise floor on r0.
		whiteNoise := fix.MaxInt(
			fix.SmulWB(fix.RShift32(autoCorr[0], 4),
				fix.FixConst32(float64(silk.ShapeWhiteNoiseFraction), 20)), 1)
		autoCorr[0] += whiteNoise

		// Reflection coefs → AR2.
		nrg := dsp.Schur64(reflCoefQ16[:], autoCorr[:], psEnc.Cmn.ShapingLPCOrder)
		lpc.K2aQ16(AR2Q24[:], reflCoefQ16[:], psEnc.Cmn.ShapingLPCOrder)

		Qnrg := -scale
		if Qnrg&1 != 0 {
			Qnrg--
			nrg >>= 1
		}
		tmp32 := fix.SqrtApprox(nrg)
		Qnrg >>= 1

		ctrl.GainsQ16[k] = fix.LShiftSat32(tmp32, 16-Qnrg)

		if psEnc.Cmn.WarpingQ16 > 0 {
			gainMultQ16 := warpedGain(AR2Q24[:], warpingQ16, psEnc.Cmn.ShapingLPCOrder)
			ctrl.GainsQ16[k] = fix.SmulWW(ctrl.GainsQ16[k], gainMultQ16)
			if ctrl.GainsQ16[k] < 0 {
				ctrl.GainsQ16[k] = 0x7FFFFFFF
			}
		}

		// Shape-synthesis BWE → AR2. Then copy and apply analysis BWE → AR1.
		dsp.BwExpander32(AR2Q24[:], psEnc.Cmn.ShapingLPCOrder, BWExp2Q16)
		copy(AR1Q24[:psEnc.Cmn.ShapingLPCOrder], AR2Q24[:psEnc.Cmn.ShapingLPCOrder])
		dsp.BwExpander32(AR1Q24[:], psEnc.Cmn.ShapingLPCOrder, BWExp1Q16)

		// Pre-emphasis from prediction-gain ratio.
		var preNrgQ30, nrgInv int32
		lpc.LPCInversePredGainQ24(&preNrgQ30, AR2Q24[:], psEnc.Cmn.ShapingLPCOrder)
		lpc.LPCInversePredGainQ24(&nrgInv, AR1Q24[:], psEnc.Cmn.ShapingLPCOrder)
		preNrgQ30 = fix.LShift32(fix.SmulWB(preNrgQ30, fix.FixConst32(0.7, 15)), 1)
		ctrl.GainsPreQ14[k] = fix.FixConst32(0.3, 14) + fix.Div32VarQ(preNrgQ30, nrgInv, 14)

		// Convert to monic warped + amplitude limit.
		limitWarpedCoefs(AR2Q24[:], AR1Q24[:], warpingQ16, fix.FixConst32(3.999, 24), psEnc.Cmn.ShapingLPCOrder)

		// Q24 → Q13 int16.
		base := k * silk.MaxShapeLPCOrder
		for i := int32(0); i < psEnc.Cmn.ShapingLPCOrder; i++ {
			ctrl.AR1Q13[base+i] = int16(fix.Sat16(fix.RShiftRound(AR1Q24[i], 11)))
			ctrl.AR2Q13[base+i] = int16(fix.Sat16(fix.RShiftRound(AR2Q24[i], 11)))
		}
	}

	// Gain tweaking for SNR / noise floor.
	gainMultQ16 := dsp.Log2Lin(-fix.SmlaWB(-fix.FixConst32(16.0, 7), SNRAdjDBQ7, fix.FixConst32(0.16, 16)))
	gainAddQ16 := dsp.Log2Lin(fix.SmlaWB(fix.FixConst32(16.0, 7),
		fix.FixConst32(float64(silk.NoiseFloorDB), 7), fix.FixConst32(0.16, 16)))
	tmp32 := dsp.Log2Lin(fix.SmlaWB(fix.FixConst32(16.0, 7),
		fix.FixConst32(float64(silk.RelativeMinGainDB), 7), fix.FixConst32(0.16, 16)))
	tmp32 = fix.SmulWW(psEnc.AvgGainQ16, tmp32)
	gainAddQ16 = fix.AddSat32(gainAddQ16, tmp32)

	for k := int32(0); k < silk.NBSubFr; k++ {
		ctrl.GainsQ16[k] = fix.SmulWW(ctrl.GainsQ16[k], gainMultQ16)
		if ctrl.GainsQ16[k] < 0 {
			ctrl.GainsQ16[k] = 0x7FFFFFFF
		}
	}
	for k := int32(0); k < silk.NBSubFr; k++ {
		ctrl.GainsQ16[k] = fix.AddPosSat32(ctrl.GainsQ16[k], gainAddQ16)
		psEnc.AvgGainQ16 = fix.AddSat32(psEnc.AvgGainQ16,
			fix.SmulWB(ctrl.GainsQ16[k]-psEnc.AvgGainQ16,
				fix.RShiftRound(fix.SmulBB(psEnc.SpeechActivityQ8,
					fix.FixConst32(float64(silk.GainSmoothingCoef), 10)), 2)))
	}

	// Fricative de-essing — applied to GainsPre rather than Gains.
	gainMultQ16 = fix.FixConst32(1.0, 16) + fix.RShiftRound(
		fix.Mla(fix.FixConst32(float64(silk.InputTilt), 26),
			ctrl.CodingQualityQ14, fix.FixConst32(float64(silk.HighRateInputTilt), 12)), 10)

	if ctrl.InputTiltQ15 <= 0 && ctrl.Cmn.Sigtype == silk.SigTypeUnvoiced {
		switch psEnc.Cmn.FsKHz {
		case 24:
			essStrengthQ15 := fix.SmulWW(-ctrl.InputTiltQ15,
				fix.SmulBB(psEnc.SpeechActivityQ8, fix.FixConst32(1.0, 8)-ctrl.SparsenessQ8))
			tmp32 = dsp.Log2Lin(fix.FixConst32(16.0, 7) -
				fix.SmulWB(essStrengthQ15,
					fix.SmulWB(fix.FixConst32(float64(silk.DeEsserCoefSWBdB), 7), fix.FixConst32(0.16, 17))))
			gainMultQ16 = fix.SmulWW(gainMultQ16, tmp32)
		case 16:
			essStrengthQ15 := fix.SmulWW(-ctrl.InputTiltQ15,
				fix.SmulBB(psEnc.SpeechActivityQ8, fix.FixConst32(1.0, 8)-ctrl.SparsenessQ8))
			tmp32 = dsp.Log2Lin(fix.FixConst32(16.0, 7) -
				fix.SmulWB(essStrengthQ15,
					fix.SmulWB(fix.FixConst32(float64(silk.DeEsserCoefWBdB), 7), fix.FixConst32(0.16, 17))))
			gainMultQ16 = fix.SmulWW(gainMultQ16, tmp32)
		}
	}
	for k := int32(0); k < silk.NBSubFr; k++ {
		ctrl.GainsPreQ14[k] = fix.SmulWB(gainMultQ16, ctrl.GainsPreQ14[k])
	}

	// LF shaping + tilt.
	strengthQ16 = fix.FixConst32(float64(silk.LowFreqShaping), 0) *
		(fix.FixConst32(1.0, 16) +
			fix.SmulBB(fix.FixConst32(float64(silk.LowQualityLowFreqShapingDecr), 1),
				ctrl.InputQualityBandsQ15[0]-fix.FixConst32(1.0, 15)))
	var TiltQ16 int32
	if ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		fsKHzInv := fix.FixConst32(0.2, 14) / psEnc.Cmn.FsKHz
		for k := int32(0); k < silk.NBSubFr; k++ {
			bQ14 := fsKHzInv + fix.FixConst32(3.0, 14)/ctrl.Cmn.PitchL[k]
			ctrl.LFShpQ14[k] = fix.LShift32(
				fix.FixConst32(1.0, 14)-bQ14-fix.SmulWB(strengthQ16, bQ14), 16)
			ctrl.LFShpQ14[k] |= int32(uint16(bQ14 - fix.FixConst32(1.0, 14)))
		}
		TiltQ16 = -fix.FixConst32(float64(silk.HPNoiseCoef), 16) -
			fix.SmulWB(fix.FixConst32(1.0, 16)-fix.FixConst32(float64(silk.HPNoiseCoef), 16),
				fix.SmulWB(fix.FixConst32(float64(silk.HarmHPNoiseCoef), 24), psEnc.SpeechActivityQ8))
	} else {
		bQ14 := int32(21299) / psEnc.Cmn.FsKHz // 1.3 in Q14
		ctrl.LFShpQ14[0] = fix.LShift32(
			fix.FixConst32(1.0, 14)-bQ14-fix.SmulWB(strengthQ16, fix.SmulWB(fix.FixConst32(0.6, 16), bQ14)), 16)
		ctrl.LFShpQ14[0] |= int32(uint16(bQ14 - fix.FixConst32(1.0, 14)))
		for k := int32(1); k < silk.NBSubFr; k++ {
			ctrl.LFShpQ14[k] = ctrl.LFShpQ14[0]
		}
		TiltQ16 = -fix.FixConst32(float64(silk.HPNoiseCoef), 16)
	}

	// Harmonic boost / shape gain.
	HarmBoostQ16 := fix.SmulWB(
		fix.SmulWB(fix.FixConst32(1.0, 17)-fix.LShift32(ctrl.CodingQualityQ14, 3), psEnc.LTPCorrQ15),
		fix.FixConst32(float64(silk.LowRateHarmonicBoost), 16))
	HarmBoostQ16 = fix.SmlaWB(HarmBoostQ16,
		fix.FixConst32(1.0, 16)-fix.LShift32(ctrl.InputQualityQ14, 2),
		fix.FixConst32(float64(silk.LowInputQualityHarmonicBoost), 16))

	var HarmShapeGainQ16 int32
	if silk.UseHarmShaping != 0 && ctrl.Cmn.Sigtype == silk.SigTypeVoiced {
		HarmShapeGainQ16 = fix.SmlaWB(fix.FixConst32(float64(silk.HarmonicShaping), 16),
			fix.FixConst32(1.0, 16)-fix.SmulWB(
				fix.FixConst32(1.0, 18)-fix.LShift32(ctrl.CodingQualityQ14, 4), ctrl.InputQualityQ14),
			fix.FixConst32(float64(silk.HighRateOrLowQualityHarmonicShaping), 16))
		HarmShapeGainQ16 = fix.SmulWB(fix.LShift32(HarmShapeGainQ16, 1),
			fix.SqrtApprox(fix.LShift32(psEnc.LTPCorrQ15, 15)))
	}

	// Smooth across subframes via SShape state.
	for k := int32(0); k < silk.NBSubFr; k++ {
		psShapeSt.HarmBoostSmthQ16 = fix.SmlaWB(psShapeSt.HarmBoostSmthQ16,
			HarmBoostQ16-psShapeSt.HarmBoostSmthQ16, fix.FixConst32(float64(silk.SubfrSmthCoef), 16))
		psShapeSt.HarmShapeGainSmthQ16 = fix.SmlaWB(psShapeSt.HarmShapeGainSmthQ16,
			HarmShapeGainQ16-psShapeSt.HarmShapeGainSmthQ16, fix.FixConst32(float64(silk.SubfrSmthCoef), 16))
		psShapeSt.TiltSmthQ16 = fix.SmlaWB(psShapeSt.TiltSmthQ16,
			TiltQ16-psShapeSt.TiltSmthQ16, fix.FixConst32(float64(silk.SubfrSmthCoef), 16))

		ctrl.HarmBoostQ14[k] = fix.RShiftRound(psShapeSt.HarmBoostSmthQ16, 2)
		ctrl.HarmShapeGainQ14[k] = fix.RShiftRound(psShapeSt.HarmShapeGainSmthQ16, 2)
		ctrl.TiltQ14[k] = fix.RShiftRound(psShapeSt.TiltSmthQ16, 2)
	}
}
