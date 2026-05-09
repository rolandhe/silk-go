package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/resampler"
)

// EncControl is SKP_SILK_SDK_EncControlStruct — the user-facing tunables
// for the encoder. Identical layout to the C SDK header so callers can
// translate field-by-field. Numeric units match the C side.
type EncControl struct {
	APISampleRate         int32 // 8000/12000/16000/24000/32000/44100/48000
	MaxInternalSampleRate int32 // 8000/12000/16000/24000
	PacketSize            int32 // packet length in samples (API rate)
	BitRate               int32 // target bitrate (bps); clamped to [Min,Max]TargetRateBPS
	PacketLossPercentage  int32 // 0..100
	Complexity            int32 // 0/1/2
	UseInBandFEC          int32 // 0/1
	UseDTX                int32 // 0/1
}

// EncoderSize — SKP_Silk_SDK_Get_Encoder_Size.
//
// Returns the number of bytes the C side allocates for an encoder. Go
// callers don't need this for sizing (the StateFIX is a Go struct, not
// raw memory), but the function is here for API parity and rough sizing
// telemetry.
func EncoderSize() int32 {
	return int32(stateFIXSizeBytes)
}

// Conservatively reported StateFIX size — rounded up so it stays valid
// across small layout changes; only used for SDK parity.
const stateFIXSizeBytes = 200000

// QueryEncoder — SKP_Silk_SDK_QueryEncoder.
//
// Read back the active control structure from a running encoder.
func QueryEncoder(psEnc *StateFIX) EncControl {
	return EncControl{
		APISampleRate:         psEnc.Cmn.APIFsHz,
		MaxInternalSampleRate: psEnc.Cmn.MaxInternalFsKHz * 1000,
		PacketSize:            (psEnc.Cmn.APIFsHz * psEnc.Cmn.PacketSizeMs) / 1000,
		BitRate:               psEnc.Cmn.TargetRateBPS,
		PacketLossPercentage:  psEnc.Cmn.PacketLossPerc,
		Complexity:            psEnc.Cmn.Complexity,
		UseInBandFEC:          psEnc.Cmn.UseInBandFEC,
		UseDTX:                psEnc.Cmn.UseDTX,
	}
}

// InitEncoder — SKP_Silk_SDK_InitEncoder.
//
// Resets the encoder state and reads back the current control struct.
// Returns 0 on success.
func InitEncoder(psEnc *StateFIX) (EncControl, int32) {
	if rc := InitEncoderFIX(psEnc); rc != 0 {
		return EncControl{}, rc
	}
	return QueryEncoder(psEnc), 0
}

// validAPISampleRate / validMaxInternalSampleRate — the canonical sets
// of input rates the SDK accepts.
func validAPISampleRate(hz int32) bool {
	switch hz {
	case 8000, 12000, 16000, 24000, 32000, 44100, 48000:
		return true
	}
	return false
}

func validMaxInternalSampleRate(hz int32) bool {
	switch hz {
	case 8000, 12000, 16000, 24000:
		return true
	}
	return false
}

// Encode — SKP_Silk_SDK_Encode.
//
// Buffer up `samplesIn` (resampling to the internal rate if needed),
// run encode_frame whenever a frame's worth has accumulated, and return
// the bytes-emitted-this-call in *nBytesOut.
//
// The encoder may need multiple input chunks before it produces a
// payload — callers should keep feeding 10/20/40 ms chunks until the
// PacketSizeMs window closes and a payload comes back.
//
// Translated from vendor/silk/src/SKP_Silk_enc_API.c.
func Encode(psEnc *StateFIX, encControl *EncControl, samplesIn []int16, nSamplesIn int32, outData []byte, nBytesOut *int32) int32 {
	if !validAPISampleRate(encControl.APISampleRate) ||
		!validMaxInternalSampleRate(encControl.MaxInternalSampleRate) {
		return silk.EncFsNotSupported
	}

	apiFsHz := encControl.APISampleRate
	maxInternalFsKHz := (encControl.MaxInternalSampleRate >> 10) + 1 // Hz → kHz
	packetSizeMs := (1000 * encControl.PacketSize) / apiFsHz

	psEnc.Cmn.APIFsHz = apiFsHz
	psEnc.Cmn.MaxInternalFsKHz = maxInternalFsKHz
	psEnc.Cmn.UseInBandFEC = encControl.UseInBandFEC

	// Input length must be a clean multiple of 10 ms and fit the slice.
	if nSamplesIn < 0 || int(nSamplesIn) > len(samplesIn) {
		return silk.EncInputInvalidNoOfSamples
	}
	input10ms := (100 * nSamplesIn) / apiFsHz
	if input10ms*apiFsHz != 100*nSamplesIn {
		return silk.EncInputInvalidNoOfSamples
	}

	targetRate := fix.Limit(encControl.BitRate, silk.MinTargetRateBPS, silk.MaxTargetRateBPS)
	if rc := ControlEncoderFIX(psEnc, packetSizeMs, encControl.PacketLossPercentage,
		encControl.UseDTX, encControl.Complexity, targetRate); rc != 0 {
		return rc
	}

	if 1000*nSamplesIn > psEnc.Cmn.PacketSizeMs*apiFsHz {
		return silk.EncInputInvalidNoOfSamples
	}

	// Detect SWB content above 8 kHz when relevant.
	if fix.MinInt(apiFsHz, 1000*maxInternalFsKHz) == 24000 &&
		psEnc.Cmn.SSWBDetect.SWBDetected == 0 &&
		psEnc.Cmn.SSWBDetect.WBDetected == 0 {
		DetectSWBInput(&psEnc.Cmn.SSWBDetect, samplesIn, nSamplesIn)
	}

	var maxBytesOut int32
	var ret int32

	for {
		nSamplesToBuffer := psEnc.Cmn.FrameLength - psEnc.Cmn.InputBufIx
		var nSamplesFromInput int32

		if apiFsHz == 1000*psEnc.Cmn.FsKHz {
			nSamplesToBuffer = fix.MinInt(nSamplesToBuffer, nSamplesIn)
			nSamplesFromInput = nSamplesToBuffer
			copy(psEnc.Cmn.InputBuf[psEnc.Cmn.InputBufIx:psEnc.Cmn.InputBufIx+nSamplesFromInput],
				samplesIn[:nSamplesFromInput])
		} else {
			nSamplesToBuffer = fix.MinInt(nSamplesToBuffer, 10*input10ms*psEnc.Cmn.FsKHz)
			nSamplesFromInput = (nSamplesToBuffer * apiFsHz) / (psEnc.Cmn.FsKHz * 1000)
			ret += resampler.Process(&psEnc.Cmn.ResamplerState,
				psEnc.Cmn.InputBuf[psEnc.Cmn.InputBufIx:],
				samplesIn, nSamplesFromInput)
		}
		samplesIn = samplesIn[nSamplesFromInput:]
		nSamplesIn -= nSamplesFromInput
		psEnc.Cmn.InputBufIx += nSamplesToBuffer

		if psEnc.Cmn.InputBufIx >= psEnc.Cmn.FrameLength {
			if maxBytesOut == 0 {
				// First time around — no payload yet, take *nBytesOut as
				// the budget and write back into maxBytesOut.
				maxBytesOut = *nBytesOut
				if rc := EncodeFrameFIX(psEnc, outData, &maxBytesOut, psEnc.Cmn.InputBuf[:]); rc != 0 {
					ret = rc
				}
			} else {
				// Already have a payload; the next frame must NOT close
				// out a new one. Pass *nBytesOut by pointer so the
				// encoder writes 0 there.
				if rc := EncodeFrameFIX(psEnc, outData, nBytesOut, psEnc.Cmn.InputBuf[:]); rc != 0 {
					ret = rc
				}
			}
			psEnc.Cmn.InputBufIx = 0
			psEnc.Cmn.ControlledSinceLastPayload = 0

			if nSamplesIn == 0 {
				break
			}
		} else {
			break
		}
	}

	*nBytesOut = maxBytesOut
	if psEnc.Cmn.UseDTX != 0 && psEnc.Cmn.InDTX != 0 {
		*nBytesOut = 0
	}
	return ret
}
