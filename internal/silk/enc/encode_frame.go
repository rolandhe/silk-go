package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/nsq"
	"github.com/rolandhe/silk-go/internal/silk/tables"
	"github.com/rolandhe/silk-go/internal/silk/vad"
)

// nsqParams pulls the FrameParams a NSQ/NSQDelDec call needs out of the
// encoder state + control. Centralizes the bookkeeping and keeps both
// call sites (main + LBRR) consistent.
func nsqParams(c *CommonState, ctrl *ControlFIX) *nsq.FrameParams {
	p := &nsq.FrameParams{
		FrameLength:       c.FrameLength,
		SubfrLength:       c.SubfrLength,
		PredictLPCOrder:   c.PredictLPCOrder,
		ShapingLPCOrder:   c.ShapingLPCOrder,
		Sigtype:           ctrl.Cmn.Sigtype,
		QuantOffsetType:   ctrl.Cmn.QuantOffsetType,
		LSFInterpFactorQ2: ctrl.Cmn.NLSFInterpCoefQ2,
		LambdaQ10:         ctrl.LambdaQ10,
		LTPScaleQ14:       ctrl.LTPScaleQ14,
		Seed:              ctrl.Cmn.Seed,
		PitchL:            ctrl.Cmn.PitchL,
		GainsQ16:          ctrl.GainsQ16,
		HarmShapeGainQ14:  ctrl.HarmShapeGainQ14,
		TiltQ14:           ctrl.TiltQ14,
		LFShpQ14:          ctrl.LFShpQ14,
	}
	return p
}

// runNSQ — dispatch to NSQDelDec when delayed-decision quantization is
// active (multi-state or warped shape), otherwise to the simpler NSQ.
// Mirrors the if/else in encode_frame_FIX.c around the NSQ call.
func runNSQ(c *CommonState, ctrl *ControlFIX, nsqState *nsq.State, xfw []int16, q []int8) {
	p := nsqParams(c, ctrl)
	if c.NStatesDelayedDecision > 1 || c.WarpingQ16 > 0 {
		var seedOut int32
		nsq.NSQDelDec(nsqState, xfw, q, p, &ctrl.PredCoefQ12, ctrl.LTPCoefQ14[:], ctrl.AR2Q13[:],
			c.WarpingQ16, c.NStatesDelayedDecision, &seedOut)
		ctrl.Cmn.Seed = seedOut
	} else {
		nsq.NSQ(nsqState, xfw, q, p, &ctrl.PredCoefQ12, ctrl.LTPCoefQ14[:], ctrl.AR2Q13[:])
	}
}

// EncodeFrameFIX — SKP_Silk_encode_frame_FIX.
//
// One frame through the full encoder. Returns 0 on success or one of the
// EncInternalError / EncPayloadBufTooShort sentinels.
//
// pCode receives at most pNBytesOut bytes; on return pNBytesOut points
// to the actual number written (0 if this frame doesn't close out a
// packet — packets aggregate PacketSizeMs of 20 ms frames).
//
// Translated from vendor/silk/src/SKP_Silk_encode_frame_FIX.c.
func EncodeFrameFIX(psEnc *StateFIX, pCode []byte, pNBytesOut *int32, pIn []int16) int32 {
	var ret int32
	var ctrl ControlFIX
	ctrl.Cmn.Seed = psEnc.Cmn.FrameCounter & 3
	psEnc.Cmn.FrameCounter++

	// Layout: x_buf  = psEnc.XBuf
	//         x_frame_off = frame_length      (start of current frame)
	xFrameOff := psEnc.Cmn.FrameLength
	resPitchLen := 2*silk.MaxFrameLength + silk.LAPitchMax
	resPitch := make([]int16, resPitchLen)
	resPitchFrameOff := psEnc.Cmn.FrameLength

	// VAD.
	var snrDBQ7 int32
	ret = vad.GetSAQ8(&psEnc.Cmn.SVAD, &psEnc.SpeechActivityQ8, &snrDBQ7,
		&ctrl.InputTiltQ15, ctrl.InputQualityBandsQ15[:], pIn, psEnc.Cmn.FrameLength)

	// HP filter (variable cutoff).
	pInHP := make([]int16, psEnc.Cmn.FrameLength)
	HPVariableCutoffFIX(psEnc, &ctrl, pInHP, pIn)

	// Optional fs-transition LP filter, into x_buf at the post-LA-shape slot.
	xLPDst := psEnc.XBuf[xFrameOff+silk.LAShapeMs*psEnc.Cmn.FsKHz:]
	LPVariableCutoff(&psEnc.Cmn.SLP, xLPDst, pInHP, psEnc.Cmn.FrameLength)

	// Pitch lags + LPC residual.
	FindPitchLagsFIX(psEnc, &ctrl, resPitch, psEnc.XBuf[:], xFrameOff)

	// Noise shape analysis (writes shape AR + gains + LF + tilt + harm).
	NoiseShapeAnalysisFIX(psEnc, &ctrl, resPitch[resPitchFrameOff:],
		psEnc.XBuf[:], xFrameOff)

	// Prefilter for noise shaper.
	xfw := make([]int16, psEnc.Cmn.FrameLength)
	PrefilterFIX(psEnc, &ctrl, xfw, psEnc.XBuf[xFrameOff:])

	// Find LPC + LTP + quantize NLSF + per-subframe residual energy.
	FindPredCoefsFIX(psEnc, &ctrl, resPitch)

	// Final gain processing (and Lambda).
	ProcessGainsFIX(psEnc, &ctrl)

	// LBRR encoding (writes psEnc.Cmn.QLBRR + an entry into the LBRR
	// payload buffer, restoring the original gains on exit).
	lbrrPayload := make([]byte, silk.MaxArithmBytes)
	nBytesLBRR := int32(silk.MaxArithmBytes)
	LBRREncodeFIX(psEnc, &ctrl, lbrrPayload, &nBytesLBRR, xfw)

	// Main NSQ.
	runNSQ(&psEnc.Cmn, &ctrl, &psEnc.Cmn.SNSQ, xfw, psEnc.Cmn.Q[:])

	// Speech activity → VAD/DTX flags.
	if psEnc.SpeechActivityQ8 < fix.FixConst32(float64(silk.SpeechActivityDTXThres), 8) {
		psEnc.Cmn.VadFlag = silk.NoVoiceActivity
		psEnc.Cmn.NoSpeechCounter++
		if psEnc.Cmn.NoSpeechCounter > silk.NoSpeechFramesBeforeDTX {
			psEnc.Cmn.InDTX = 1
		}
		if psEnc.Cmn.NoSpeechCounter > silk.MaxConsecutiveDTX+silk.NoSpeechFramesBeforeDTX {
			psEnc.Cmn.NoSpeechCounter = silk.NoSpeechFramesBeforeDTX
			psEnc.Cmn.InDTX = 0
		}
	} else {
		psEnc.Cmn.NoSpeechCounter = 0
		psEnc.Cmn.InDTX = 0
		psEnc.Cmn.VadFlag = silk.VoiceActivity
	}

	// Range coder init for first frame in packet.
	if psEnc.Cmn.NFramesInPayloadBuf == 0 {
		psEnc.Cmn.SRC.EncInit()
		psEnc.Cmn.NBytesInPayloadBuf = 0
	}

	// Encode all per-frame parameters.
	EncodeParameters(&psEnc.Cmn, &ctrl.Cmn, &psEnc.Cmn.SRC, psEnc.Cmn.Q[:])

	// Slide x_buf forward by one frame.
	keepLen := psEnc.Cmn.FrameLength + silk.LAShapeMs*psEnc.Cmn.FsKHz
	copy(psEnc.XBuf[:keepLen], psEnc.XBuf[psEnc.Cmn.FrameLength:psEnc.Cmn.FrameLength+keepLen])

	psEnc.Cmn.PrevSigtype = ctrl.Cmn.Sigtype
	psEnc.Cmn.PrevLag = ctrl.Cmn.PitchL[silk.NBSubFr-1]
	psEnc.Cmn.FirstFrameAfterReset = 0

	if psEnc.Cmn.SRC.Error != 0 {
		psEnc.Cmn.NFramesInPayloadBuf = 0
	} else {
		psEnc.Cmn.NFramesInPayloadBuf++
	}

	// Either close out the packet or just emit a "more frames" terminator.
	var nBytes int32
	if psEnc.Cmn.NFramesInPayloadBuf*silk.FrameLengthMs >= psEnc.Cmn.PacketSizeMs {
		lbrrIdx := (psEnc.Cmn.OldestLBRRIdx + 1) & silk.LBRRIdxMask

		frameTerminator := int32(silk.LastFrame)
		if psEnc.Cmn.LBRRBuffer[lbrrIdx].Usage == silk.AddLBRRToPlus1 {
			frameTerminator = silk.LBRRVer1
		}
		if psEnc.Cmn.LBRRBuffer[psEnc.Cmn.OldestLBRRIdx].Usage == silk.AddLBRRToPlus2 {
			frameTerminator = silk.LBRRVer2
			lbrrIdx = psEnc.Cmn.OldestLBRRIdx
		}

		psEnc.Cmn.SRC.Encode(frameTerminator, tables.FrameTermination_CDF[:])

		_, nBytes = psEnc.Cmn.SRC.GetLength()

		if *pNBytesOut >= nBytes {
			psEnc.Cmn.SRC.EncWrapUp()
			copy(pCode[:nBytes], psEnc.Cmn.SRC.Buffer[:nBytes])

			if frameTerminator > silk.MoreFrames &&
				*pNBytesOut >= nBytes+psEnc.Cmn.LBRRBuffer[lbrrIdx].NBytes {
				copy(pCode[nBytes:], psEnc.Cmn.LBRRBuffer[lbrrIdx].Payload[:psEnc.Cmn.LBRRBuffer[lbrrIdx].NBytes])
				nBytes += psEnc.Cmn.LBRRBuffer[lbrrIdx].NBytes
			}
			*pNBytesOut = nBytes

			// Update FEC ring with this frame's LBRR.
			oldest := &psEnc.Cmn.LBRRBuffer[psEnc.Cmn.OldestLBRRIdx]
			copy(oldest.Payload[:nBytesLBRR], lbrrPayload[:nBytesLBRR])
			oldest.NBytes = nBytesLBRR
			oldest.Usage = ctrl.Cmn.LBRRUsage
			psEnc.Cmn.OldestLBRRIdx = (psEnc.Cmn.OldestLBRRIdx + 1) & silk.LBRRIdxMask
		} else {
			*pNBytesOut = 0
			nBytes = 0
			ret = silk.EncPayloadBufTooShort
		}
		psEnc.Cmn.NFramesInPayloadBuf = 0
	} else {
		*pNBytesOut = 0
		psEnc.Cmn.SRC.Encode(silk.MoreFrames, tables.FrameTermination_CDF[:])
		_, nBytes = psEnc.Cmn.SRC.GetLength()
	}

	if psEnc.Cmn.SRC.Error != 0 {
		ret = silk.EncInternalError
	}

	// Channel-buffering tracker.
	psEnc.BufferedInChannelMs += (8 * 1000 * (nBytes - psEnc.Cmn.NBytesInPayloadBuf)) / psEnc.Cmn.TargetRateBPS
	psEnc.BufferedInChannelMs -= silk.FrameLengthMs
	psEnc.BufferedInChannelMs = fix.Limit(psEnc.BufferedInChannelMs, 0, 100)
	psEnc.Cmn.NBytesInPayloadBuf = nBytes

	if psEnc.SpeechActivityQ8 > fix.FixConst32(float64(silk.WBDetectActiveSpeechLevelThres), 8) {
		psEnc.Cmn.SSWBDetect.ActiveSpeechMs = fix.AddPosSat32(
			psEnc.Cmn.SSWBDetect.ActiveSpeechMs, silk.FrameLengthMs)
	}
	return ret
}

// LBRREncodeFIX — SKP_Silk_LBRR_encode_FIX.
//
// Builds a redundant copy of the current frame at slightly higher gain
// (per `LBRR_GainIncreases`), runs it through a separate NSQ instance
// (`SNSQLBRR`), and emits its parameter stream into a separate range
// coder. The output payload goes to `pCode`; the original gains are
// restored on exit so the main encoder can continue unaffected.
//
// Translated from vendor/silk/src/SKP_Silk_encode_frame_FIX.c.
func LBRREncodeFIX(psEnc *StateFIX, ctrl *ControlFIX, pCode []byte, pNBytesOut *int32, xfw []int16) {
	LBRRCtrlFIX(psEnc, &ctrl.Cmn)

	if psEnc.Cmn.LBRREnabled == 0 {
		return
	}

	// Snapshot original gains/scale so we can restore them.
	tempGainsIndices := ctrl.Cmn.GainsIndices
	tempGainsQ16 := ctrl.GainsQ16
	typeOffset := psEnc.Cmn.TypeOffsetPrev
	LTPScaleIndex := ctrl.Cmn.LTPScaleIndex

	// Per-rate threshold: above this we encode the residual; otherwise
	// just the parameters.
	var rateOnlyParams int32
	switch psEnc.Cmn.FsKHz {
	case 8:
		rateOnlyParams = 13500
	case 12:
		rateOnlyParams = 15500
	case 16:
		rateOnlyParams = 17500
	case 24:
		rateOnlyParams = 19500
	}

	if psEnc.Cmn.Complexity > 0 && psEnc.Cmn.TargetRateBPS > rateOnlyParams {
		if psEnc.Cmn.NFramesInPayloadBuf == 0 {
			psEnc.Cmn.SNSQLBRR = psEnc.Cmn.SNSQ
			psEnc.Cmn.LBRRPrevLastGainIndex = psEnc.SShape.LastGainIndex
			ctrl.Cmn.GainsIndices[0] = ctrl.Cmn.GainsIndices[0] + psEnc.Cmn.LBRRGainIncreases
			ctrl.Cmn.GainsIndices[0] = fix.Limit(ctrl.Cmn.GainsIndices[0], 0, silk.NLevelsQGain-1)
		}
		// Re-derive quantized gains so they match what the decoder will see.
		dsp.GainsDequant(ctrl.GainsQ16[:], ctrl.Cmn.GainsIndices[:],
			&psEnc.Cmn.LBRRPrevLastGainIndex, psEnc.Cmn.NFramesInPayloadBuf)

		runNSQ(&psEnc.Cmn, ctrl, &psEnc.Cmn.SNSQLBRR, xfw, psEnc.Cmn.QLBRR[:])
	} else {
		for i := range psEnc.Cmn.QLBRR[:psEnc.Cmn.FrameLength] {
			psEnc.Cmn.QLBRR[i] = 0
		}
		ctrl.Cmn.LTPScaleIndex = 0
	}

	if psEnc.Cmn.NFramesInPayloadBuf == 0 {
		psEnc.Cmn.SRCLBRR.EncInit()
		psEnc.Cmn.NBytesInPayloadBuf = 0
	}

	EncodeParameters(&psEnc.Cmn, &ctrl.Cmn, &psEnc.Cmn.SRCLBRR, psEnc.Cmn.QLBRR[:])

	var nFramesInBuf int32
	if psEnc.Cmn.SRCLBRR.Error != 0 {
		nFramesInBuf = 0
	} else {
		nFramesInBuf = psEnc.Cmn.NFramesInPayloadBuf + 1
	}

	if nFramesInBuf*silk.FrameLengthMs >= psEnc.Cmn.PacketSizeMs {
		frameTerminator := int32(silk.LastFrame)
		psEnc.Cmn.SRCLBRR.Encode(frameTerminator, tables.FrameTermination_CDF[:])

		_, nBytes := psEnc.Cmn.SRCLBRR.GetLength()
		if *pNBytesOut >= nBytes {
			psEnc.Cmn.SRCLBRR.EncWrapUp()
			copy(pCode[:nBytes], psEnc.Cmn.SRCLBRR.Buffer[:nBytes])
			*pNBytesOut = nBytes
		} else {
			*pNBytesOut = 0
		}
	} else {
		*pNBytesOut = 0
		psEnc.Cmn.SRCLBRR.Encode(silk.MoreFrames, tables.FrameTermination_CDF[:])
	}

	// Restore gains, LTP scale, type-offset.
	ctrl.Cmn.GainsIndices = tempGainsIndices
	ctrl.GainsQ16 = tempGainsQ16
	ctrl.Cmn.LTPScaleIndex = LTPScaleIndex
	psEnc.Cmn.TypeOffsetPrev = typeOffset
}
