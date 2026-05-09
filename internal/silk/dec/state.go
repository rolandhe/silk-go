package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/resampler"
)

// PLCStruct mirrors SKP_Silk_PLC_struct from vendor/silk/src/SKP_Silk_structs.h.
type PLCStruct struct {
	PitchLQ8        int32                   // pitch lag for voiced concealment
	LTPCoefQ14      [silk.LTPOrder]int16    // LTP coefficients
	PrevLPCQ12      [silk.MaxLPCOrder]int16 // previous LPC
	LastFrameLost   int32
	RandSeed        int32
	RandScaleQ14    int16
	ConcEnergy      int32
	ConcEnergyShift int32
	PrevLTPScaleQ14 int16
	PrevGainQ16     [silk.NBSubFr]int32
	FsKHz           int32
}

// CNGStruct mirrors SKP_Silk_CNG_struct.
type CNGStruct struct {
	CNGExcBufQ10   [silk.MaxFrameLength]int32
	CNGSmthNLSFQ15 [silk.MaxLPCOrder]int32
	CNGSynthState  [silk.MaxLPCOrder]int32
	CNGSmthGainQ16 int32
	RandSeed       int32
	FsKHz          int32
}

// State mirrors SKP_Silk_decoder_state. The C source layouts sRC at offset 0
// because some range-coder paths cast the void* directly; Go has typed
// pointers so we just embed by value.
type State struct {
	SRC rangecoder.State

	PrevInvGainQ16 int32

	// History buffers.
	SLTPQ16 [2 * silk.MaxFrameLength]int32
	SLPCQ14 [silk.MaxFrameLength/silk.NBSubFr + silk.MaxLPCOrder]int32
	ExcQ10  [silk.MaxFrameLength]int32
	ResQ10  [silk.MaxFrameLength]int32
	OutBuf  [2 * silk.MaxFrameLength]int16

	LagPrev               int32
	LastGainIndex         int32
	LastGainIndexEnhLayer int32
	TypeOffsetPrev        int32

	HPState [silk.DecHPOrder]int32
	HPA     []int16 // points into tables (Dec_A_HP_*)
	HPB     []int16 // points into tables (Dec_B_HP_*)

	FsKHz             int32
	PrevAPISampleRate int32
	FrameLength       int32
	SubfrLength       int32
	LPCOrder          int32

	PrevNLSFQ15 [silk.MaxLPCOrder]int32

	FirstFrameAfterReset int32

	// Multi-frame packet buffering.
	NBytesLeft                int32
	NFramesDecoded            int32
	NFramesInPacket           int32
	MoreInternalDecoderFrames int32
	FrameTermination          int32

	ResamplerState resampler.State

	// Voiced/unvoiced NLSF codebook pointers.
	NLSFCB [2]*nlsf.CBStruct

	// Inband-FEC investigation.
	VadFlag         int32
	NoFECCounter    int32
	InbandFECOffset int32

	// CNG.
	SCNG CNGStruct

	// PLC.
	LossCnt     int32
	PrevSigtype int32
	SPLC        PLCStruct
}

// Control mirrors SKP_Silk_decoder_control.
type Control struct {
	PitchL      [silk.NBSubFr]int32
	GainsQ16    [silk.NBSubFr]int32
	Seed        int32
	PredCoefQ12 [2][silk.MaxLPCOrder]int16
	LTPCoefQ14  [silk.LTPOrder * silk.NBSubFr]int16
	LTPScaleQ14 int32

	// Quantization indices.
	PERIndex         int32
	RateLevelIndex   int32
	QuantOffsetType  int32
	Sigtype          int32
	NLSFInterpCoefQ2 int32
}
