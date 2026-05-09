// Package dec implements the SILK decoder. This file consolidates the public
// decoder API translated from
//
//	vendor/silk/src/SKP_Silk_dec_API.c            — public entry points
//	vendor/silk/src/SKP_Silk_create_init_destroy.c — SKP_Silk_init_decoder
//
// The C side mixes a "create" (allocator) function and an "init" (state reset)
// function. In Go the State is a value type the caller allocates as &State{},
// so we expose only InitDecoder and a DecoderSize helper for SDK parity.
package dec

import (
	"unsafe"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/resampler"
)

// VERSION is returned by GetVersion. Mirrors the static "1.0.9.6" string in
// SKP_Silk_SDK_get_version (vendor/silk/src/SKP_Silk_dec_API.c).
const VERSION = "1.0.9.6"

// DecoderSize returns the byte size of a decoder State. The C SDK exposed this
// so a caller could allocate a flat buffer; in Go callers allocate &State{}
// directly, but we keep the helper for API parity.
//
// Mirrors SKP_Silk_SDK_Get_Decoder_Size.
func DecoderSize() int32 {
	return int32(unsafe.Sizeof(State{}))
}

// InitDecoder resets a decoder State. It zeros the struct, sets the internal
// sample rate to 24 kHz, primes prev_inv_gain_Q16, and resets CNG and PLC.
//
// Mirrors SKP_Silk_SDK_InitDecoder + SKP_Silk_init_decoder.
func InitDecoder(s *State) int32 {
	*s = State{}

	// Set sampling rate to 24 kHz, and init non-zero values.
	DecoderSetFs(s, 24)

	// Used to deactivate e.g. LSF interpolation and fluctuation reduction.
	s.FirstFrameAfterReset = 1
	s.PrevInvGainQ16 = 65536

	// Reset CNG and PLC sub-states.
	CNGReset(s)
	PLCReset(s)

	return 0
}

// Decode decodes a single SILK frame. lostFlag = 0 for a normal packet, 1 to
// indicate packet loss (PLC concealment). On packet loss inData/nBytesIn are
// ignored. Output samples are written to samplesOut at ctrl.APISampleRate;
// the number of output samples produced is written to *nSamplesOut.
//
// On the first frame of a packet ctrl.FrameSize, FramesPerPacket,
// MoreInternalDecoderFrames and InBandFECOffset are populated.
//
// Mirrors SKP_Silk_SDK_Decode.
func Decode(s *State, ctrl *silk.DecControl, lostFlag int32, inData []byte, nBytesIn int32,
	samplesOut []int16, nSamplesOut *int32) int32 {

	var ret int32 = 0
	var usedBytes int32

	// Internal scratch for upsampling: max API kHz times 20 ms.
	var samplesOutInternal [silk.MaxAPIFsKHz * silk.FrameLengthMs]int16

	// Choose where decode_frame writes: when the internal rate exceeds the
	// API rate the resampler downsamples into samplesOut, and decode_frame
	// must therefore use a larger scratch buffer.
	pSamplesOutInternal := samplesOut
	if s.FsKHz*1000 > ctrl.APISampleRate {
		pSamplesOutInternal = samplesOutInternal[:]
	}

	// Test if first frame in payload.
	if s.MoreInternalDecoderFrames == 0 {
		s.NFramesDecoded = 0 // count frames in packet
	}

	if s.MoreInternalDecoderFrames == 0 && // first frame in packet
		lostFlag == 0 && // not packet loss
		nBytesIn > silk.MaxArithmBytes { // too long payload
		// Avoid trying to decode a too large packet.
		lostFlag = 1
		ret = silk.DecPayloadTooLarge
	}

	// Save previous internal sample frequency.
	prevFsKHz := s.FsKHz

	// Call decoder for one frame.
	ret += DecodeFrame(s, pSamplesOutInternal, nSamplesOut, inData, nBytesIn,
		lostFlag, &usedBytes)

	if usedBytes != 0 { // only when not a packet loss
		if s.NBytesLeft > 0 && s.FrameTermination == silk.MoreFrames && s.NFramesDecoded < 5 {
			// More frames in this payload.
			s.MoreInternalDecoderFrames = 1
		} else {
			// Last frame in payload.
			s.MoreInternalDecoderFrames = 0
			s.NFramesInPacket = s.NFramesDecoded

			// Track inband FEC usage.
			if s.VadFlag == silk.VoiceActivity {
				switch s.FrameTermination {
				case silk.LastFrame:
					s.NoFECCounter++
					if s.NoFECCounter > silk.NoLBRRThres {
						s.InbandFECOffset = 0
					}
				case silk.LBRRVer1:
					s.InbandFECOffset = 1 // FEC info with 1 packet delay
					s.NoFECCounter = 0
				case silk.LBRRVer2:
					s.InbandFECOffset = 2 // FEC info with 2 packets delay
					s.NoFECCounter = 0
				}
			}
		}
	}

	if silk.MaxAPIFsKHz*1000 < ctrl.APISampleRate || 8000 > ctrl.APISampleRate {
		ret = silk.DecInvalidSamplingFrequency
		return ret
	}

	// Resample if needed.
	if s.FsKHz*1000 != ctrl.APISampleRate {
		var samplesOutTmp [silk.MaxAPIFsKHz * silk.FrameLengthMs]int16

		// Copy to a tmp buffer because the resampler writes into samplesOut.
		copy(samplesOutTmp[:*nSamplesOut], pSamplesOutInternal[:*nSamplesOut])

		// (Re-)initialise resampler when switching internal frequency.
		if prevFsKHz != s.FsKHz || s.PrevAPISampleRate != ctrl.APISampleRate {
			ret = resampler.Init(&s.ResamplerState, s.FsKHz*1000, ctrl.APISampleRate)
		}

		// Resample to API rate.
		ret += resampler.Process(&s.ResamplerState, samplesOut, samplesOutTmp[:], *nSamplesOut)

		// Update output sample count.
		*nSamplesOut = (*nSamplesOut * ctrl.APISampleRate) / (s.FsKHz * 1000)
	} else if prevFsKHz*1000 > ctrl.APISampleRate {
		// Internal rate just dropped to match API rate but decode_frame still
		// wrote into the internal scratch on entry — copy across.
		copy(samplesOut[:*nSamplesOut], pSamplesOutInternal[:*nSamplesOut])
	}

	s.PrevAPISampleRate = ctrl.APISampleRate

	// Copy parameters out for the caller.
	ctrl.FrameSize = ctrl.APISampleRate / 50
	ctrl.FramesPerPacket = s.NFramesInPacket
	ctrl.InBandFECOffset = s.InbandFECOffset
	ctrl.MoreInternalDecoderFrames = s.MoreInternalDecoderFrames

	return ret
}

// SearchForLBRR scans an encoded packet for inband-FEC (LBRR) information at
// the given lostOffset (1 or 2 packets behind). On hit the LBRR payload is
// copied into LBRRData and *nLBRRBytes set; on miss *nLBRRBytes is 0.
//
// Mirrors SKP_Silk_SDK_search_for_LBRR.
func SearchForLBRR(inData []byte, nBytesIn int32, lostOffset int32, LBRRData []byte, nLBRRBytes *int32) {
	if lostOffset < 1 || lostOffset > silk.MaxLBRRDelay {
		// No useful FEC in this packet.
		*nLBRRBytes = 0
		return
	}

	// Local decoder state to avoid disturbing a running decoder.
	var sDec State
	var sDecCtrl Control
	var tempQ [silk.MaxFrameLength]int32

	sDec.NFramesDecoded = 0
	sDec.FsKHz = 0 // force update of LPC_order etc
	sDec.LossCnt = 0
	// PrevNLSFQ15 already zero from State{} initialisation.

	sDec.SRC.DecInit(inData[:nBytesIn])

	for {
		DecodeParameters(&sDec, &sDecCtrl, tempQ[:], false)

		if sDec.SRC.Error != 0 {
			// Corrupt stream.
			*nLBRRBytes = 0
			return
		}
		if (sDec.FrameTermination-1)&lostOffset != 0 && sDec.FrameTermination > 0 && sDec.NBytesLeft >= 0 {
			// Wanted FEC is present.
			*nLBRRBytes = sDec.NBytesLeft
			copy(LBRRData[:sDec.NBytesLeft], inData[nBytesIn-sDec.NBytesLeft:nBytesIn])
			return
		}
		if sDec.NBytesLeft > 0 && sDec.FrameTermination == silk.MoreFrames {
			sDec.NFramesDecoded++
			continue
		}
		// No more frames and no FEC found.
		*nLBRRBytes = 0
		return
	}
}

// GetTOC parses the table-of-contents fields of a SILK packet without
// affecting any active decoder. It returns frames-in-packet, internal sample
// rate, inband-LBRR mode, and per-frame VAD/sigtype flags. On a corrupt
// stream toc.Corrupt is set and the rest is zeroed.
//
// Mirrors SKP_Silk_SDK_get_TOC.
func GetTOC(inData []byte, nBytesIn int32, toc *silk.TOC) {
	var sDec State
	var sDecCtrl Control
	var tempQ [silk.MaxFrameLength]int32

	sDec.NFramesDecoded = 0
	sDec.FsKHz = 0 // force update of LPC_order etc
	sDec.SRC.DecInit(inData[:nBytesIn])

	toc.Corrupt = 0
	for {
		DecodeParameters(&sDec, &sDecCtrl, tempQ[:], false)

		if sDec.NFramesDecoded < int32(len(toc.VadFlags)) {
			toc.VadFlags[sDec.NFramesDecoded] = sDec.VadFlag
			toc.SigtypeFlags[sDec.NFramesDecoded] = sDecCtrl.Sigtype
		}

		if sDec.SRC.Error != 0 {
			toc.Corrupt = 1
			break
		}

		if sDec.NBytesLeft > 0 && sDec.FrameTermination == silk.MoreFrames {
			sDec.NFramesDecoded++
			continue
		}
		break
	}
	if toc.Corrupt != 0 || sDec.FrameTermination == silk.MoreFrames ||
		sDec.NFramesInPacket > silk.MaxFramesPerPacket {
		// Corrupt packet.
		*toc = silk.TOC{}
		toc.Corrupt = 1
	} else {
		toc.FramesInPacket = sDec.NFramesDecoded + 1
		toc.FsKHz = sDec.FsKHz
		if sDec.FrameTermination == silk.LastFrame {
			toc.InbandLBRR = sDec.FrameTermination
		} else {
			toc.InbandLBRR = sDec.FrameTermination - 1
		}
	}
}

// GetVersion returns the SILK SDK version string.
//
// Mirrors SKP_Silk_SDK_get_version.
func GetVersion() string {
	return VERSION
}
