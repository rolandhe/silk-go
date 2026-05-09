package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
)

// DecodeFrame mirrors SKP_Silk_decode_frame from
// vendor/silk/src/SKP_Silk_decode_frame.c.
//
// Decodes a single SILK frame from the bitstream pCode (length nBytes) into
// pOut. The number of output samples is written to *pN, and the number of
// bytes consumed from pCode to *decBytes. The action parameter comes from the
// jitter buffer: 0 means decode normally, 1 means the packet is lost/corrupt
// and the frame should be concealed via PLC. Returns 0 on success or a
// negative silk.Dec* error code on payload error.
func DecodeFrame(s *State, pOut []int16, pN *int32, pCode []byte, nBytes int32, action int32, decBytes *int32) int32 {
	var sDecCtrl Control
	var pulses [silk.MaxFrameLength]int32

	L := s.FrameLength
	sDecCtrl.LTPScaleQ14 = 0

	// Safety check on frame length is implicit; assert dropped.

	// Decode frame if packet is not lost.
	*decBytes = 0
	var ret int32 = 0
	if action == 0 {
		fsKHzOld := s.FsKHz
		if s.NFramesDecoded == 0 {
			// Initialize range decoder state.
			s.SRC.DecInit(pCode[:nBytes])
		}

		// Decode parameters and pulse signal.
		DecodeParameters(s, &sDecCtrl, pulses[:], true)

		if s.SRC.Error != 0 {
			s.NBytesLeft = 0

			action = 1 // PLC operation
			// Revert fs if changed in DecodeParameters.
			DecoderSetFs(s, fsKHzOld)

			// Avoid crashing.
			*decBytes = s.SRC.BufferLength

			if s.SRC.Error == silk.RangeCoderDecPayloadTooLong {
				ret = silk.DecPayloadTooLarge
			} else {
				ret = silk.DecPayloadError
			}
		} else {
			*decBytes = s.SRC.BufferLength - s.NBytesLeft
			s.NFramesDecoded++

			// Update lengths. Sampling frequency could have changed.
			L = s.FrameLength

			// Run inverse NSQ.
			DecodeCore(s, &sDecCtrl, pOut, pulses[:])

			// Update PLC state.
			PLC(s, &sDecCtrl, pOut, L, action)

			s.LossCnt = 0
			s.PrevSigtype = sDecCtrl.Sigtype

			// A frame has been decoded without errors.
			s.FirstFrameAfterReset = 0
		}
	}

	// Generate concealment frame if packet is lost or corrupt.
	if action == 1 {
		PLC(s, &sDecCtrl, pOut, L, action)
	}

	// Update output buffer.
	copy(s.OutBuf[:L], pOut[:L])

	// Ensure smooth connection of extrapolated and good frames.
	PLCGlueFrames(s, &sDecCtrl, pOut, L)

	// Comfort noise generation / estimation.
	CNG(s, &sDecCtrl, pOut, L)

	// HP filter output.
	Biquad(pOut, s.HPB, s.HPA, s.HPState[:], pOut, L)

	// Set output frame length.
	*pN = L

	// Update some decoder state variables.
	s.LagPrev = sDecCtrl.PitchL[silk.NBSubFr-1]

	return ret
}
