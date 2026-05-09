package silk

// Public control structures. Mirror vendor/silk/interface/SKP_Silk_control.h.

// EncControl is the encoder control struct passed to Encode. Field meanings:
//
//   APISampleRate         Input sample rate (Hz). 8000/12000/16000/24000.
//   MaxInternalSampleRate Max internal sample rate (Hz). 8000/12000/16000/24000.
//   PacketSize            Samples per packet — equivalent of 20/40/60/80/100 ms.
//   BitRate               Active-speech bitrate (bps); internally limited.
//   PacketLossPercentage  Uplink loss percent (0–100).
//   Complexity            0 lowest, 2 highest.
//   UseInBandFEC          0/1.
//   UseDTX                0/1.
type EncControl struct {
	APISampleRate         int32
	MaxInternalSampleRate int32
	PacketSize            int32
	BitRate               int32
	PacketLossPercentage  int32
	Complexity            int32
	UseInBandFEC          int32
	UseDTX                int32
}

// DecControl is the decoder control struct (in/out). FrameSize, FramesPerPacket,
// MoreInternalDecoderFrames, InBandFECOffset are populated by Decode.
type DecControl struct {
	APISampleRate              int32
	FrameSize                  int32
	FramesPerPacket            int32
	MoreInternalDecoderFrames  int32
	InBandFECOffset            int32
}

// SilkMaxFramesPerPacket — name kept for parity with the C header.
const SilkMaxFramesPerPacket = MaxFramesPerPacket

// TOC mirrors SKP_Silk_TOC_struct from SKP_Silk_SDK_API.h.
type TOC struct {
	FramesInPacket int32
	FsKHz          int32
	InbandLBRR     int32
	Corrupt        int32
	VadFlags       [SilkMaxFramesPerPacket]int32
	SigtypeFlags   [SilkMaxFramesPerPacket]int32
}
