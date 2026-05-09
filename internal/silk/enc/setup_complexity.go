package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// SetupComplexity — SKP_Silk_setup_complexity.
//
// Map the user-facing complexity setting (0/1/2 = low/medium/high) to the
// internal knobs that gate work in pitch estimation, NLSF MSVQ, delayed
// decision NSQ, and noise shaping. Returns 0 on success, or
// EncInvalidComplexitySetting if the value is out of range.
//
// Translated from vendor/silk/src/SKP_Silk_setup_complexity.h.
func SetupComplexity(c *CommonState, complexity int32) int32 {
	if silk.LowComplexityOnly != 0 && complexity != 0 {
		return silk.EncInvalidComplexitySetting
	}

	switch {
	case complexity == 0 || silk.LowComplexityOnly != 0:
		c.Complexity = 0
		c.PitchEstimationComplexity = silk.PitchEstComplexityLCMode
		c.PitchEstimationThresholdQ16 = fix.FixConst32(float64(silk.FindPitchCorrelationThresholdLCMode), 16)
		c.PitchEstimationLPCOrder = 6
		c.ShapingLPCOrder = 8
		c.LAShape = 3 * c.FsKHz
		c.NStatesDelayedDecision = 1
		c.UseInterpolatedNLSFs = 0
		c.LTPQuantLowComplexity = 1
		c.NLSFMSVQSurvivors = silk.MaxNLSFMSVQSurvivorsLCMode
		c.WarpingQ16 = 0
	case complexity == 1:
		c.Complexity = 1
		c.PitchEstimationComplexity = silk.PitchEstComplexityMCMode
		c.PitchEstimationThresholdQ16 = fix.FixConst32(float64(silk.FindPitchCorrelationThresholdMCMode), 16)
		c.PitchEstimationLPCOrder = 12
		c.ShapingLPCOrder = 12
		c.LAShape = 5 * c.FsKHz
		c.NStatesDelayedDecision = 2
		c.UseInterpolatedNLSFs = 0
		c.LTPQuantLowComplexity = 0
		c.NLSFMSVQSurvivors = silk.MaxNLSFMSVQSurvivorsMCMode
		c.WarpingQ16 = c.FsKHz * fix.FixConst32(float64(silk.WarpingMultiplier), 16)
	case complexity == 2:
		c.Complexity = 2
		c.PitchEstimationComplexity = silk.PitchEstComplexityHCMode
		c.PitchEstimationThresholdQ16 = fix.FixConst32(float64(silk.FindPitchCorrelationThresholdHCMode), 16)
		c.PitchEstimationLPCOrder = 16
		c.ShapingLPCOrder = 16
		c.LAShape = 5 * c.FsKHz
		c.NStatesDelayedDecision = silk.MaxDelDecStates
		c.UseInterpolatedNLSFs = 1
		c.LTPQuantLowComplexity = 0
		c.NLSFMSVQSurvivors = silk.MaxNLSFMSVQSurvivors
		c.WarpingQ16 = c.FsKHz * fix.FixConst32(float64(silk.WarpingMultiplier), 16)
	default:
		return silk.EncInvalidComplexitySetting
	}

	// Clamp pitch-estimation order against the prediction LPC order — pitch
	// uses a sub-order whitening filter so it can't exceed the main one.
	c.PitchEstimationLPCOrder = fix.MinInt(c.PitchEstimationLPCOrder, c.PredictLPCOrder)
	c.ShapeWinLength = 5*c.FsKHz + 2*c.LAShape
	return 0
}
