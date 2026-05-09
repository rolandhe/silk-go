package silk

// Tuning parameters from vendor/silk/src/SKP_Silk_tuning_parameters.h. The
// SILK fixed-point codebase uses these as float32 inputs to SKP_FIX_CONST,
// so we mirror them as float32 here and rely on the fix.FixConst helpers.

const (
	FindPitchWhiteNoiseFraction = float32(1e-3)
	FindPitchBandwithExpansion  = float32(0.99)

	FindPitchCorrelationThresholdHCMode = float32(0.7)
	FindPitchCorrelationThresholdMCMode = float32(0.75)
	FindPitchCorrelationThresholdLCMode = float32(0.8)

	FindLPCCondFac = float32(2.5e-5)
	FindLPCChirp   = float32(0.99995)

	FindLTPCondFac = float32(1e-5)
	LTPDamping     = float32(0.01)
	LTPSmoothing   = float32(0.1)

	MULTPQuantNB  = float32(0.03)
	MULTPQuantMB  = float32(0.025)
	MULTPQuantWB  = float32(0.02)
	MULTPQuantSWB = float32(0.016)

	VariableHPSmthCoef1     = float32(0.1)
	VariableHPSmthCoef2     = float32(0.015)
	VariableHPMinFreq       = float32(80.0)
	VariableHPMaxFreq       = float32(150.0)
	VariableHPMaxDeltaFreq  = float32(0.4)

	WBDetectActiveSpeechLevelThres = float32(0.7)
	SpeechActivityDTXThres         = float32(0.1)
	LBRRSpeechActivityThres        = float32(0.5)

	BGSNRDecrDB        = float32(4.0)
	HarmSNRIncrDB      = float32(2.0)
	SparseSNRIncrDB    = float32(2.0)
	SparsenessThresholdQntOffset = float32(0.75)

	WarpingMultiplier        = float32(0.015)
	ShapeWhiteNoiseFraction  = float32(1e-5)
	BandwidthExpansion       = float32(0.95)
	LowRateBandwidthExpansionDelta = float32(0.01)

	DeEsserCoefSWBdB = float32(2.0)
	DeEsserCoefWBdB  = float32(1.0)

	LowRateHarmonicBoost          = float32(0.1)
	LowInputQualityHarmonicBoost  = float32(0.1)

	HarmonicShaping                          = float32(0.3)
	HighRateOrLowQualityHarmonicShaping      = float32(0.2)

	HPNoiseCoef     = float32(0.3)
	HarmHPNoiseCoef = float32(0.35)

	InputTilt          = float32(0.05)
	HighRateInputTilt  = float32(0.1)

	LowFreqShaping            = float32(3.0)
	LowQualityLowFreqShapingDecr = float32(0.5)

	NoiseFloorDB        = float32(4.0)
	RelativeMinGainDB   = float32(-50.0)

	GainSmoothingCoef = float32(1e-3)
	SubfrSmthCoef     = float32(0.4)

	LambdaOffset            = float32(1.2)
	LambdaSpeechAct         = float32(-0.3)
	LambdaDelayedDecisions  = float32(-0.05)
	LambdaInputQuality      = float32(-0.2)
	LambdaCodingQuality     = float32(-0.1)
	LambdaQuantOffset       = float32(1.5)

	NLSFMSVQSurvMaxRelRD = float32(0.1)
)
