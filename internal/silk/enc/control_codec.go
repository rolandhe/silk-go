package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
	"github.com/rolandhe/silk-go/internal/silk/resampler"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// ControlEncoderFIX — SKP_Silk_control_encoder_FIX.
//
// Per-payload control entry point. Picks the internal sampling rate, sets
// up the resampler against the requested API rate, locks in the packet
// size and complexity, derives an SNR target from the bitrate, validates
// the loss-rate / DTX / FEC inputs, and arms the LBRR machinery.
//
// Idempotent within a single payload: once `controlled_since_last_payload`
// is set, only an API-rate change retriggers the resampler init.
//
// Translated from vendor/silk/src/SKP_Silk_control_codec_FIX.c.
func ControlEncoderFIX(psEnc *StateFIX, packetSizeMs, packetLossPerc, dtxEnabled, complexity int32, targetRateBPS int32) int32 {
	var ret int32

	if psEnc.Cmn.ControlledSinceLastPayload != 0 {
		if psEnc.Cmn.APIFsHz != psEnc.Cmn.PrevAPIFsHz && psEnc.Cmn.FsKHz > 0 {
			// API rate changed mid-payload — re-init the resampler against
			// the new rate but don't redo packet/fs/etc. setup.
			ret += setupResamplersFIX(psEnc, psEnc.Cmn.FsKHz)
		}
		return ret
	}

	fsKHz := ControlAudioBandwidth(&psEnc.Cmn, targetRateBPS)

	ret += setupResamplersFIX(psEnc, fsKHz)
	ret += setupPacketSizeFIX(psEnc, packetSizeMs)
	ret += setupFsFIX(psEnc, fsKHz)
	ret += SetupComplexity(&psEnc.Cmn, complexity)
	ret += setupRateFIX(psEnc, targetRateBPS)

	if packetLossPerc < 0 || packetLossPerc > 100 {
		ret = silk.EncInvalidLossRate
	}
	psEnc.Cmn.PacketLossPerc = packetLossPerc

	ret += setupLBRRFIX(psEnc)

	if dtxEnabled < 0 || dtxEnabled > 1 {
		ret = silk.EncInvalidDtxSetting
	}
	psEnc.Cmn.UseDTX = dtxEnabled
	psEnc.Cmn.ControlledSinceLastPayload = 1

	return ret
}

// LBRRCtrlFIX — SKP_Silk_LBRR_ctrl_FIX.
//
// Decide whether the current frame's LBRR copy should be carried forward
// in the next packet. Currently a simple gate: only attach LBRR when
// speech is fairly active *and* the far-end is reporting losses.
//
// Translated from vendor/silk/src/SKP_Silk_control_codec_FIX.c.
func LBRRCtrlFIX(psEnc *StateFIX, ctrl *CommonControl) {
	if psEnc.Cmn.LBRREnabled == 0 {
		ctrl.LBRRUsage = silk.NoLBRR
		return
	}

	usage := int32(silk.NoLBRR)
	if psEnc.SpeechActivityQ8 > fix.FixConst32(float64(silk.LBRRSpeechActivityThres), 8) &&
		psEnc.Cmn.PacketLossPerc > silk.LBRRLossThres {
		usage = silk.AddLBRRToPlus1
	}
	ctrl.LBRRUsage = usage
}

// setupResamplersFIX — SKP_Silk_setup_resamplers_FIX.
//
// Reinitializes the API↔internal-rate resampler whenever the internal rate
// or the API rate changes. When the internal rate is dropping mid-encoding
// we first upsample the carry-over buffer back to API rate via a
// throwaway resampler, then re-resample it down at the new rate so the
// look-ahead history stays continuous.
func setupResamplersFIX(psEnc *StateFIX, fsKHz int32) int32 {
	var ret int32

	if psEnc.Cmn.FsKHz != fsKHz || psEnc.Cmn.PrevAPIFsHz != psEnc.Cmn.APIFsHz {
		if psEnc.Cmn.FsKHz == 0 {
			ret += resampler.Init(&psEnc.Cmn.ResamplerState, psEnc.Cmn.APIFsHz, fsKHz*1000)
		} else {
			// Worst-case temporary upsampling buffer: 8 → 48 kHz factor 6.
			const xBufLen = (2*silk.MaxFrameLength + silk.LAShapeMax) * (silk.MaxAPIFsKHz / 8)
			var xBufAPI [xBufLen]int16

			nSamplesTemp := fix.LShift32(psEnc.Cmn.FrameLength, 1) + silk.LAShapeMs*psEnc.Cmn.FsKHz

			if fsKHz*1000 < psEnc.Cmn.APIFsHz && psEnc.Cmn.FsKHz != 0 {
				// Upsample x_buf to API rate via a throwaway resampler.
				var tmp resampler.State
				ret += resampler.Init(&tmp, psEnc.Cmn.FsKHz*1000, psEnc.Cmn.APIFsHz)
				ret += resampler.Process(&tmp, xBufAPI[:], psEnc.XBuf[:], nSamplesTemp)

				// Re-derive sample count at API rate.
				nSamplesTemp = (nSamplesTemp * psEnc.Cmn.APIFsHz) / (psEnc.Cmn.FsKHz * 1000)

				ret += resampler.Init(&psEnc.Cmn.ResamplerState, psEnc.Cmn.APIFsHz, fsKHz*1000)
			} else {
				copy(xBufAPI[:nSamplesTemp], psEnc.XBuf[:nSamplesTemp])
			}

			if 1000*fsKHz != psEnc.Cmn.APIFsHz {
				// Resample buffered data back to fsKHz so the stateful
				// resampler picks up where we left off.
				ret += resampler.Process(&psEnc.Cmn.ResamplerState, psEnc.XBuf[:], xBufAPI[:], nSamplesTemp)
			}
		}
	}

	psEnc.Cmn.PrevAPIFsHz = psEnc.Cmn.APIFsHz
	return ret
}

// setupPacketSizeFIX — SKP_Silk_setup_packetsize_FIX.
//
// Validate the packet size and, if it changed, reset the LBRR ring (slots
// from a different packet-size cadence don't line up).
func setupPacketSizeFIX(psEnc *StateFIX, packetSizeMs int32) int32 {
	switch packetSizeMs {
	case 20, 40, 60, 80, 100:
	default:
		return silk.EncPacketSizeNotSupported
	}
	if packetSizeMs != psEnc.Cmn.PacketSizeMs {
		psEnc.Cmn.PacketSizeMs = packetSizeMs
		LBRRReset(&psEnc.Cmn)
	}
	return 0
}

// setupFsFIX — SKP_Silk_setup_fs_FIX.
//
// React to an internal-rate change: clear shape/prefilter/predict/NSQ
// state, reseed bookkeeping, point the NLSF codebook pointers at the
// matching order, and lock in rate-dependent thresholds. Also flips
// FsKHzChanged for downstream code that needs to react.
func setupFsFIX(psEnc *StateFIX, fsKHz int32) int32 {
	if psEnc.Cmn.FsKHz == fsKHz {
		return 0
	}

	psEnc.SShape = ShapeStateFIX{}
	psEnc.SPrefilt = PrefilterStateFIX{}
	psEnc.SPred = PredictStateFIX{}
	psEnc.Cmn.SNSQ = NSQState{}
	for i := range psEnc.Cmn.SNSQLBRR.Xq {
		psEnc.Cmn.SNSQLBRR.Xq[i] = 0
	}
	for i := range psEnc.Cmn.LBRRBuffer {
		psEnc.Cmn.LBRRBuffer[i] = LBRRStruct{}
	}

	psEnc.Cmn.SLP.InLPState[0] = 0
	psEnc.Cmn.SLP.InLPState[1] = 0
	if psEnc.Cmn.SLP.Mode == 1 {
		psEnc.Cmn.SLP.TransitionFrameNo = 1
	} else {
		psEnc.Cmn.SLP.TransitionFrameNo = 0
	}

	psEnc.Cmn.InputBufIx = 0
	psEnc.Cmn.NFramesInPayloadBuf = 0
	psEnc.Cmn.NBytesInPayloadBuf = 0
	psEnc.Cmn.OldestLBRRIdx = 0
	psEnc.Cmn.TargetRateBPS = 0 // forces SNR_dB recompute

	for i := range psEnc.SPred.PrevNLSFqQ15 {
		psEnc.SPred.PrevNLSFqQ15[i] = 0
	}

	psEnc.Cmn.PrevLag = 100
	psEnc.Cmn.PrevSigtype = silk.SigTypeUnvoiced
	psEnc.Cmn.FirstFrameAfterReset = 1
	psEnc.SPrefilt.LagPrev = 100
	psEnc.SShape.LastGainIndex = 1
	psEnc.Cmn.SNSQ.LagPrev = 100
	psEnc.Cmn.SNSQ.PrevInvGainQ16 = 65536
	psEnc.Cmn.SNSQLBRR.PrevInvGainQ16 = 65536

	psEnc.Cmn.FsKHz = fsKHz
	if fsKHz == 8 {
		psEnc.Cmn.PredictLPCOrder = silk.MinLPCOrder
		psEnc.Cmn.NLSFCB[0] = &nlsf.CB0_10
		psEnc.Cmn.NLSFCB[1] = &nlsf.CB1_10
	} else {
		psEnc.Cmn.PredictLPCOrder = silk.MaxLPCOrder
		psEnc.Cmn.NLSFCB[0] = &nlsf.CB0_16
		psEnc.Cmn.NLSFCB[1] = &nlsf.CB1_16
	}
	psEnc.Cmn.FrameLength = silk.FrameLengthMs * fsKHz
	psEnc.Cmn.SubfrLength = psEnc.Cmn.FrameLength / silk.NBSubFr
	psEnc.Cmn.LAPitch = silk.LAPitchMs * fsKHz
	psEnc.SPred.MinPitchLag = 3 * fsKHz
	psEnc.SPred.MaxPitchLag = 18 * fsKHz
	psEnc.SPred.PitchLPCWinLength = silk.FindPitchLPCWinMs * fsKHz

	switch fsKHz {
	case 24:
		psEnc.MuLTPQ8 = fix.FixConst32(float64(silk.MULTPQuantSWB), 8)
		psEnc.Cmn.BitrateThresholdUp = 0x7FFFFFFF
		psEnc.Cmn.BitrateThresholdDown = silk.SWB2WBBitrateBPS
	case 16:
		psEnc.MuLTPQ8 = fix.FixConst32(float64(silk.MULTPQuantWB), 8)
		psEnc.Cmn.BitrateThresholdUp = silk.WB2SWBBitrateBPS
		psEnc.Cmn.BitrateThresholdDown = silk.WB2MBBitrateBPS
	case 12:
		psEnc.MuLTPQ8 = fix.FixConst32(float64(silk.MULTPQuantMB), 8)
		psEnc.Cmn.BitrateThresholdUp = silk.MB2WBBitrateBPS
		psEnc.Cmn.BitrateThresholdDown = silk.MB2NBBitrateBPS
	default: // 8 kHz
		psEnc.MuLTPQ8 = fix.FixConst32(float64(silk.MULTPQuantNB), 8)
		psEnc.Cmn.BitrateThresholdUp = silk.NB2MBBitrateBPS
		psEnc.Cmn.BitrateThresholdDown = 0
	}
	psEnc.Cmn.FsKHzChanged = 1
	return 0
}

// setupRateFIX — SKP_Silk_setup_rate_FIX.
//
// Translate the target bitrate into an SNR-dB target by linearly
// interpolating between rows of the per-rate table that matches the
// current internal rate.
func setupRateFIX(psEnc *StateFIX, targetRateBPS int32) int32 {
	if targetRateBPS == psEnc.Cmn.TargetRateBPS {
		return 0
	}
	psEnc.Cmn.TargetRateBPS = targetRateBPS

	var rateTable []int32
	switch psEnc.Cmn.FsKHz {
	case 8:
		rateTable = tables.TargetRate_table_NB[:]
	case 12:
		rateTable = tables.TargetRate_table_MB[:]
	case 16:
		rateTable = tables.TargetRate_table_WB[:]
	default:
		rateTable = tables.TargetRate_table_SWB[:]
	}
	for k := int32(1); k < silk.TargetRateTabSize; k++ {
		if targetRateBPS <= rateTable[k] {
			fracQ6 := fix.LShift32(targetRateBPS-rateTable[k-1], 6) / (rateTable[k] - rateTable[k-1])
			psEnc.SNRdBQ7 = fix.LShift32(tables.SNR_table_Q1[k-1], 6) +
				fracQ6*(tables.SNR_table_Q1[k]-tables.SNR_table_Q1[k-1])
			break
		}
	}
	return 0
}

// setupLBRRFIX — SKP_Silk_setup_LBRR_FIX.
//
// Decide whether to keep LBRR (in-band FEC) on for the upcoming packet.
// Below a per-rate threshold LBRR is forced off; above it, the gain
// increase used for the LBRR copy is set inversely to the loss rate.
func setupLBRRFIX(psEnc *StateFIX) int32 {
	if psEnc.Cmn.UseInBandFEC < 0 || psEnc.Cmn.UseInBandFEC > 1 {
		return silk.EncInvalidInbandFecSetting
	}
	psEnc.Cmn.LBRREnabled = psEnc.Cmn.UseInBandFEC

	var threshBPS int32
	switch psEnc.Cmn.FsKHz {
	case 8:
		threshBPS = silk.InbandFecMinRateBPS - 9000
	case 12:
		threshBPS = silk.InbandFecMinRateBPS - 6000
	case 16:
		threshBPS = silk.InbandFecMinRateBPS - 3000
	default:
		threshBPS = silk.InbandFecMinRateBPS
	}

	if psEnc.Cmn.TargetRateBPS >= threshBPS {
		// G = 8 - 0.5 * loss%; below 0 we just clamp.
		psEnc.Cmn.LBRRGainIncreases = fix.MaxInt(8-fix.RShift32(psEnc.Cmn.PacketLossPerc, 1), 0)

		if psEnc.Cmn.LBRREnabled != 0 && psEnc.Cmn.PacketLossPerc > silk.LBRRLossThres {
			// 6.0 (Q8) − Gain×128 — keeps mean bitrate ≈ no-FEC.
			psEnc.InBandFECSNRCompQ8 = fix.FixConst32(6.0, 8) - fix.LShift32(psEnc.Cmn.LBRRGainIncreases, 7)
		} else {
			psEnc.InBandFECSNRCompQ8 = 0
			psEnc.Cmn.LBRREnabled = 0
		}
	} else {
		psEnc.InBandFECSNRCompQ8 = 0
		psEnc.Cmn.LBRREnabled = 0
	}
	return 0
}
