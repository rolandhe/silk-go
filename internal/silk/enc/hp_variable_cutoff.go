package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// Constants from the C source.
const (
	hpRadiansConstantQ19    int32 = 1482 // 0.45 * 2π / 1000 in Q19
	hpLog2VariableHPMinFreq int32 = 809  // log(80) in Q7
)

// HPVariableCutoffFIX — SKP_Silk_HP_variable_cutoff_FIX.
//
// Pitch-adaptive high-pass filter. The cutoff frequency tracks the lower
// bound of the speaker's pitch range, smoothed across frames. After the
// smoother updates, builds biquad coefficients for a {1,-2,1}/{1,-2r·…,r²}
// HP filter and applies it via BiquadAlt.
//
// Translated from vendor/silk/src/SKP_Silk_HP_variable_cutoff_FIX.c.
func HPVariableCutoffFIX(psEnc *StateFIX, psEncCtrl *ControlFIX, out, in []int16) {
	// Estimate low end of pitch frequency range when previous frame was
	// voiced — otherwise just keep the smoothers as-is.
	if psEnc.Cmn.PrevSigtype == silk.SigTypeVoiced {
		pitchFreqHzQ16 := fix.LShift32(psEnc.Cmn.FsKHz*1000, 16) / psEnc.Cmn.PrevLag
		pitchFreqLogQ7 := dsp.Lin2Log(pitchFreqHzQ16) - (16 << 7)

		// Quality-based correction.
		qualityQ15 := psEncCtrl.InputQualityBandsQ15[0]
		pitchFreqLogQ7 -= fix.SmulWB(
			fix.SmulWB(fix.LShift32(qualityQ15, 2), qualityQ15),
			pitchFreqLogQ7-hpLog2VariableHPMinFreq)
		pitchFreqLogQ7 += fix.RShift32(fix.FixConst32(0.6, 15)-qualityQ15, 9)

		deltaFreqQ7 := pitchFreqLogQ7 - fix.RShift32(psEnc.VariableHPSmth1Q15, 8)
		if deltaFreqQ7 < 0 {
			// Track the minimum more aggressively when pitch drops.
			deltaFreqQ7 *= 3
		}

		// Limit outliers.
		limit := fix.FixConst32(float64(silk.VariableHPMaxDeltaFreq), 7)
		deltaFreqQ7 = fix.Limit(deltaFreqQ7, -limit, limit)

		psEnc.VariableHPSmth1Q15 = fix.SmlaWB(
			psEnc.VariableHPSmth1Q15,
			fix.LShift32(psEnc.SpeechActivityQ8, 1)*deltaFreqQ7,
			fix.FixConst32(float64(silk.VariableHPSmthCoef1), 16))
	}

	// Second smoother runs every frame.
	psEnc.VariableHPSmth2Q15 = fix.SmlaWB(
		psEnc.VariableHPSmth2Q15,
		psEnc.VariableHPSmth1Q15-psEnc.VariableHPSmth2Q15,
		fix.FixConst32(float64(silk.VariableHPSmthCoef2), 16))

	// Convert to Hz, clamp.
	psEncCtrl.PitchFreqLowHz = dsp.Log2Lin(fix.RShift32(psEnc.VariableHPSmth2Q15, 8))
	psEncCtrl.PitchFreqLowHz = fix.Limit(
		psEncCtrl.PitchFreqLowHz,
		fix.FixConst32(float64(silk.VariableHPMinFreq), 0),
		fix.FixConst32(float64(silk.VariableHPMaxFreq), 0))

	// Cutoff in radians.
	FcQ19 := (hpRadiansConstantQ19 * psEncCtrl.PitchFreqLowHz) / psEnc.Cmn.FsKHz

	rQ28 := fix.FixConst32(1.0, 28) - fix.FixConst32(0.92, 9)*FcQ19

	// b = r * [1, -2, 1]
	BQ28 := [3]int32{rQ28, fix.LShift32(-rQ28, 1), rQ28}

	rQ22 := fix.RShift32(rQ28, 6)
	AQ28 := [2]int32{
		fix.SmulWW(rQ22, fix.SmulWW(FcQ19, FcQ19)-fix.FixConst32(2.0, 22)),
		fix.SmulWW(rQ22, rQ22),
	}

	dsp.BiquadAlt(in, BQ28[:], AQ28[:], psEnc.Cmn.InHPState[:], out, psEnc.Cmn.FrameLength)
}
