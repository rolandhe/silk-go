package silk

// Constants ported from vendor/silk/src/SKP_Silk_define.h.

const (
	MaxFramesPerPacket = 5

	// Bitrate limits.
	MinTargetRateBPS = 5000
	MaxTargetRateBPS = 100000

	// Mode-transition bitrates.
	SWB2WBBitrateBPS = 25000
	WB2SWBBitrateBPS = 30000
	WB2MBBitrateBPS  = 14000
	MB2WBBitrateBPS  = 18000
	MB2NBBitrateBPS  = 10000
	NB2MBBitrateBPS  = 14000

	// Hysteresis for lowering internal sampling frequency.
	AccumBitsDiffThreshold = 30000000
	TargetRateTabSize      = 8

	// DTX.
	NoSpeechFramesBeforeDTX = 5
	MaxConsecutiveDTX       = 20

	UseLBRR = 1

	NoLBRRThres  = 10
	MaxLBRRDelay = 2
	LBRRIdxMask  = 1

	InbandFecMinRateBPS = 18000
	LBRRLossThres       = 1

	NoLBRR         = 0
	AddLBRRToPlus1 = 1
	AddLBRRToPlus2 = 2

	LastFrame  = 0
	MoreFrames = 1
	LBRRVer1   = 2
	LBRRVer2   = 3
	ExtLayer   = 4

	NBSOS                       = 3
	HP8KHzThres                 = 10
	ConsecSWBSamplesThres       = 480 * 15
	WBDetectActiveSpeechMSThres = 15000

	LowComplexityOnly = 0

	SwitchTransitionFiltering = 1

	DecHPOrder = 2

	MaxFsKHz    = 24
	MaxAPIFsKHz = 48

	SigTypeVoiced   = 0
	SigTypeUnvoiced = 1

	NoVoiceActivity = 0
	VoiceActivity   = 1

	FrameLengthMs  = 20
	MaxFrameLength = FrameLengthMs * MaxFsKHz

	LAPitchMs  = 2
	LAPitchMax = LAPitchMs * MaxFsKHz

	FindPitchLPCWinMs  = 20 + (LAPitchMs << 1)
	FindPitchLPCWinMax = FindPitchLPCWinMs * MaxFsKHz

	MaxFindPitchLPCOrder = 16

	PitchEstMinComplex = 0
	PitchEstMidComplex = 1
	PitchEstMaxComplex = 2

	PitchEstComplexityHCMode = PitchEstMaxComplex
	PitchEstComplexityMCMode = PitchEstMidComplex
	PitchEstComplexityLCMode = PitchEstMinComplex

	// Pitch-estimator dimensions (vendor/silk/src/SKP_Silk_common_pitch_est_defines.h).
	PitchEstMaxFsKHz               = 24
	PitchEstFrameLengthMs          = 40
	PitchEstMaxFrameLength         = PitchEstFrameLengthMs * PitchEstMaxFsKHz
	PitchEstMaxFrameLengthSt1      = PitchEstMaxFrameLength >> 2
	PitchEstMaxFrameLengthSt2      = PitchEstMaxFrameLength >> 1
	PitchEstMaxLagMs               = 18
	PitchEstMinLagMs               = 2
	PitchEstMaxLag                 = PitchEstMaxLagMs * PitchEstMaxFsKHz
	PitchEstMinLag                 = PitchEstMinLagMs * PitchEstMaxFsKHz
	PitchEstNBSubFr                = 4
	PitchEstDSrchLength            = 24
	PitchEstMaxDecimateStateLength = 7
	PitchEstNBStage3Lags           = 5
	PitchEstNBCbksStage2           = 3
	PitchEstNBCbksStage2Ext        = 11
	PitchEstNBCbksStage3Max        = 34
	PitchEstNBCbksStage3Mid        = 24
	PitchEstNBCbksStage3Min        = 16

	// Bias factors (vendor/silk/src/SKP_Silk_pitch_est_defines.h).
	PitchEstShortLagBiasQ15    = 6554  // 0.2 in Q15 — log-domain shortlag bias
	PitchEstPrevLagBiasQ15     = 6554  // prev-lag bias
	PitchEstFlatContourBiasQ20 = 52429 // 0.05 in Q20

	LAShapeMs      = 5
	LAShapeMax     = LAShapeMs * MaxFsKHz
	ShapeLPCWinMax = 15 * MaxFsKHz

	MaxArithmBytes = 1024

	MinQGainDB        = 6
	MaxQGainDB        = 86
	NLevelsQGain      = 64
	MaxDeltaGainQuant = 40
	MinDeltaGainQuant = -4

	OffsetVLQ10  = 32
	OffsetVHQ10  = 100
	OffsetUVLQ10 = 100
	OffsetUVHQ10 = 256

	MaxLPCStabilizeIterations = 20

	MaxLPCOrder = 16
	MinLPCOrder = 10

	LTPOrder = 5

	NBLTPCBKs = 3

	NBSubFr = 4

	UseHarmShaping   = 1
	MaxShapeLPCOrder = 16
	HarmShapeFIRTaps = 3

	MaxDelDecStates = 4

	LTPBufLength = 512
	LTPMask      = LTPBufLength - 1

	DecisionDelay     = 32
	DecisionDelayMask = DecisionDelay - 1

	ShellCodecFrameLength = 16
	MaxNBShellBlocks      = MaxFrameLength / ShellCodecFrameLength

	NRateLevels = 10
	MaxPulses   = 18

	MaxMatrixSize = MaxLPCOrder

	HighPassInput = 1

	VADNBands = 4

	VADInternalSubframesLog2 = 2
	VADInternalSubframes     = 1 << VADInternalSubframesLog2

	VADNoiseLevelSmoothCoefQ16 = 1024
	VADNoiseLevelsBias         = 50

	VADNegativeOffsetQ5 = 128
	VADSNRFactorQ16     = 45000

	VADSNRSmoothCoefQ18 = 4096

	NLSFMSVQMaxCBStages               = 10
	NLSFMSVQMaxVectorsInStage         = 128
	NLSFMSVQMaxVectorsInStageTwoToEnd = 16

	NLSFMSVQFluctuationReduction = 1
	MaxNLSFMSVQSurvivors         = 16
	MaxNLSFMSVQSurvivorsLCMode   = 2
	MaxNLSFMSVQSurvivorsMCMode   = 4

	BWEAfterLossQ16 = 63570

	CNGBufMaskMax  = 255
	CNGGainSmthQ16 = 4634
	CNGNLSFSmthQ16 = 16348

	TransitionTimeUpMs   = 5120
	TransitionTimeDownMs = 2560
	TransitionNB         = 3
	TransitionNA         = 2
	TransitionIntNum     = 5
	TransitionFramesUp   = TransitionTimeUpMs / FrameLengthMs
	TransitionFramesDown = TransitionTimeDownMs / FrameLengthMs
	TransitionIntStepsUp = TransitionFramesUp / (TransitionIntNum - 1)
	TransitionIntStepsDn = TransitionFramesDown / (TransitionIntNum - 1)
)

// Derived from the C #if guards in SKP_Silk_define.h. Computed by hand:
//
//	NLSFMSVQMaxVectorsInStage = 128, MaxNLSFMSVQSurvivorsLCMode * NLSFMSVQMaxVectorsInStageTwoToEnd = 2*16 = 32 → 128 > 32, take NLSFMSVQMaxVectorsInStage.
//	NLSFMSVQMaxVectorsInStage = 128, MaxNLSFMSVQSurvivors * NLSFMSVQMaxVectorsInStageTwoToEnd = 16*16 = 256 → 128 < 256, take the product.
//	MaxLPCOrder = 16, DecisionDelay = 32 → 16 < 32, take DecisionDelay.
const (
	NLSFMSVQTreeSearchMaxVectorsEvaluatedLCMode = NLSFMSVQMaxVectorsInStage
	NLSFMSVQTreeSearchMaxVectorsEvaluated       = MaxNLSFMSVQSurvivors * NLSFMSVQMaxVectorsInStageTwoToEnd
	NSQLPCBufLength                             = DecisionDelay
)
