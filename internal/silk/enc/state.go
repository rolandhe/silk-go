// Package enc holds the SILK encoder state structures and the (forthcoming)
// fixed-point encode-frame pipeline. State layout mirrors SKP_Silk_*_FIX
// from the C source; see vendor/silk/src/SKP_Silk_structs.h and
// vendor/silk/src/SKP_Silk_structs_FIX.h.
package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
	"github.com/rolandhe/silk-go/internal/silk/nsq"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/resampler"
	"github.com/rolandhe/silk-go/internal/silk/vad"
)

// NSQState — SKP_Silk_nsq_state. Re-exported from the nsq package so the
// encoder can use it without an import cycle on the enc package itself.
type NSQState = nsq.State

// LBRRStruct — SKP_SILK_LBRR_struct (low-bitrate redundant payload slot).
type LBRRStruct struct {
	Payload [silk.MaxArithmBytes]uint8
	NBytes  int32
	Usage   int32
}

// DetectSWBState — SKP_Silk_detect_SWB_state.
type DetectSWBState struct {
	SHP8kHz             [silk.NBSOS][2]int32
	ConsecSmplsAboveThr int32
	ActiveSpeechMs      int32
	SWBDetected         int32
	WBDetected          int32
}

// LPState — SKP_Silk_LP_state. Variable-cutoff LP filter used during
// fs-switching transitions.
type LPState struct {
	InLPState         [2]int32
	TransitionFrameNo int32
	Mode              int32
}

// ShapeStateFIX — SKP_Silk_shape_state_FIX (noise-shape analysis state).
type ShapeStateFIX struct {
	LastGainIndex        int32
	HarmBoostSmthQ16     int32
	HarmShapeGainSmthQ16 int32
	TiltSmthQ16          int32
}

// PrefilterStateFIX — SKP_Silk_prefilter_state_FIX.
type PrefilterStateFIX struct {
	SLTPShp       [silk.LTPBufLength]int16
	SARShp        [silk.MaxShapeLPCOrder + 1]int32 // Q14
	SLTPShpBufIdx int32
	SLFARShpQ12   int32
	SLFMAShpQ12   int32
	SHarmHP       int32
	RandSeed      int32
	LagPrev       int32
}

// PredictStateFIX — SKP_Silk_predict_state_FIX (pitch+LPC analysis state).
type PredictStateFIX struct {
	PitchLPCWinLength int32
	MinPitchLag       int32
	MaxPitchLag       int32
	PrevNLSFqQ15      [silk.MaxLPCOrder]int32
}

// CommonState — SKP_Silk_encoder_state. The "shared" fields, originally
// referenced via the `sCmn` member in C; here we embed it directly into
// StateFIX so encoder code can reach common fields without an extra hop.
type CommonState struct {
	SRC      rangecoder.State // range coder (main payload)
	SRCLBRR  rangecoder.State // range coder (LBRR redundancy)
	SNSQ     NSQState         // noise-shape quantizer
	SNSQLBRR NSQState         // noise-shape quantizer (LBRR)

	InHPState [2]int32  // input high-pass filter state (HIGH_PASS_INPUT)
	SLP       LPState   // fs-transition low-pass filter (SWITCH_TRANSITION_FILTERING)
	SVAD      vad.State // VAD state

	LBRRPrevLastGainIndex       int32
	PrevSigtype                 int32
	TypeOffsetPrev              int32
	PrevLag                     int32
	PrevLagIndex                int32
	APIFsHz                     int32
	PrevAPIFsHz                 int32
	MaxInternalFsKHz            int32
	FsKHz                       int32
	FsKHzChanged                int32
	FrameLength                 int32
	SubfrLength                 int32
	LAPitch                     int32
	LAShape                     int32
	ShapeWinLength              int32
	TargetRateBPS               int32
	PacketSizeMs                int32
	PacketLossPerc              int32
	FrameCounter                int32
	Complexity                  int32
	NStatesDelayedDecision      int32
	UseInterpolatedNLSFs        int32
	ShapingLPCOrder             int32
	PredictLPCOrder             int32
	PitchEstimationComplexity   int32
	PitchEstimationLPCOrder     int32
	PitchEstimationThresholdQ16 int32
	LTPQuantLowComplexity       int32
	NLSFMSVQSurvivors           int32
	FirstFrameAfterReset        int32
	ControlledSinceLastPayload  int32
	WarpingQ16                  int32

	InputBuf            [silk.MaxFrameLength]int16
	InputBufIx          int32
	NFramesInPayloadBuf int32
	NBytesInPayloadBuf  int32

	FramesSinceOnset int32

	NLSFCB [2]*nlsf.CBStruct

	LBRRBuffer        [silk.MaxLBRRDelay]LBRRStruct
	OldestLBRRIdx     int32
	UseInBandFEC      int32
	LBRREnabled       int32
	LBRRGainIncreases int32

	BitrateDiff          int32
	BitrateThresholdUp   int32
	BitrateThresholdDown int32

	ResamplerState resampler.State

	NoSpeechCounter int32
	UseDTX          int32
	InDTX           int32
	VadFlag         int32

	SSWBDetect DetectSWBState

	Q     [silk.MaxFrameLength]int8 // pulse buffer (main)
	QLBRR [silk.MaxFrameLength]int8 // pulse buffer (LBRR)
}

// StateFIX — SKP_Silk_encoder_state_FIX. Embeds CommonState; the C source
// reaches through `psEnc->sCmn`, while Go code reaches through `psEnc.Cmn`.
type StateFIX struct {
	Cmn CommonState

	VariableHPSmth1Q15 int32
	VariableHPSmth2Q15 int32

	SShape   ShapeStateFIX
	SPrefilt PrefilterStateFIX
	SPred    PredictStateFIX

	XBuf [2*silk.MaxFrameLength + silk.LAShapeMax]int16

	LTPCorrQ15                int32
	MuLTPQ8                   int32
	SNRdBQ7                   int32
	AvgGainQ16                int32
	AvgGainQ16OneBitPerSample int32
	BufferedInChannelMs       int32
	SpeechActivityQ8          int32

	PrevLTPredCodGainQ7 int32
	HPLTPredCodGainQ7   int32

	InBandFECSNRCompQ8 int32
}

// CommonControl — SKP_Silk_encoder_control. Per-frame control parameters
// derived during encoding and consumed by the entropy coder.
type CommonControl struct {
	LagIndex         int32
	ContourIndex     int32
	PERIndex         int32
	LTPIndex         [silk.NBSubFr]int32
	NLSFIndices      [silk.NLSFMSVQMaxCBStages]int32
	NLSFInterpCoefQ2 int32
	GainsIndices     [silk.NBSubFr]int32
	Seed             int32
	LTPScaleIndex    int32
	RateLevelIndex   int32
	QuantOffsetType  int32
	Sigtype          int32

	PitchL [silk.NBSubFr]int32

	LBRRUsage int32
}

// ControlFIX — SKP_Silk_encoder_control_FIX.
type ControlFIX struct {
	Cmn CommonControl

	GainsQ16    [silk.NBSubFr]int32
	PredCoefQ12 [2][silk.MaxLPCOrder]int16
	LTPCoefQ14  [silk.LTPOrder * silk.NBSubFr]int16
	LTPScaleQ14 int32

	AR1Q13           [silk.NBSubFr * silk.MaxShapeLPCOrder]int16
	AR2Q13           [silk.NBSubFr * silk.MaxShapeLPCOrder]int16
	LFShpQ14         [silk.NBSubFr]int32 // packs two int16 coefs per int32
	GainsPreQ14      [silk.NBSubFr]int32
	HarmBoostQ14     [silk.NBSubFr]int32
	TiltQ14          [silk.NBSubFr]int32
	HarmShapeGainQ14 [silk.NBSubFr]int32
	LambdaQ10        int32
	InputQualityQ14  int32
	CodingQualityQ14 int32
	PitchFreqLowHz   int32
	CurrentSNRdBQ7   int32

	SparsenessQ8         int32
	PredGainQ16          int32
	LTPredCodGainQ7      int32
	InputQualityBandsQ15 [silk.VADNBands]int32
	InputTiltQ15         int32
	ResNrg               [silk.NBSubFr]int32
	ResNrgQ              [silk.NBSubFr]int32
}
