package ltp

import (
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// FrameLengthMs mirrors FRAME_LENGTH_MS (= 20) from SKP_Silk_define.h.
const FrameLengthMs = 20

// scaleCtrlNbThresholds mirrors NB_THRESHOLDS in SKP_Silk_LTP_scale_ctrl_FIX.c.
const scaleCtrlNbThresholds = 11

// ltpScaleThresholdsQ15 — trained thresholds for picking the LTP scale.
// Mirrors the static table in SKP_Silk_LTP_scale_ctrl_FIX.c.
var ltpScaleThresholdsQ15 = [scaleCtrlNbThresholds]int32{
	31129, 26214, 16384, 13107, 9830, 6554,
	4915, 3276, 2621, 2458, 0,
}

// ScaleCtrlState holds the encoder-state fields touched by ScaleCtrl. The
// full encoder state lands in stage F; until then this minimal struct keeps
// ScaleCtrl callable and testable in isolation.
//
// Field mapping to the C source (vendor/silk/src/SKP_Silk_LTP_scale_ctrl_FIX.c):
//
//	LTPredCodGainQ7        ↔ psEncCtrl->LTPredCodGain_Q7        (input)
//	HPLTPredCodGainQ7      ↔ psEnc->HPLTPredCodGain_Q7          (in/out)
//	PrevLTPredCodGainQ7    ↔ psEnc->prevLTPredCodGain_Q7        (in/out)
//	PacketLossPerc         ↔ psEnc->sCmn.PacketLoss_perc        (input)
//	NFramesInPayloadBuf    ↔ psEnc->sCmn.nFramesInPayloadBuf    (input)
//	PacketSizeMs           ↔ psEnc->sCmn.PacketSize_ms          (input)
//	LTPScaleIndex          ↔ psEncCtrl->sCmn.LTP_scaleIndex     (output)
//	LTPScaleQ14            ↔ psEncCtrl->LTP_scale_Q14           (output)
type ScaleCtrlState struct {
	LTPredCodGainQ7     int32
	HPLTPredCodGainQ7   int32
	PrevLTPredCodGainQ7 int32
	PacketLossPerc      int32
	NFramesInPayloadBuf int32
	PacketSizeMs        int32

	LTPScaleIndex int32
	LTPScaleQ14   int32
}

// ScaleCtrl — SKP_Silk_LTP_scale_ctrl_FIX.
//
// Decide the LTP scaling level (0/1/2) for the next frame based on
// predicted coding gain and packet-loss conditions. Output goes into
// state.LTPScaleIndex (0..2) and state.LTPScaleQ14 (looked up in the
// LTPScales_table).
//
// Translated from vendor/silk/src/SKP_Silk_LTP_scale_ctrl_FIX.c.
func ScaleCtrl(state *ScaleCtrlState) {
	// 1st-order high-pass on LTPredCodGain.
	delta := state.LTPredCodGainQ7 - state.PrevLTPredCodGainQ7
	if delta < 0 {
		delta = 0
	}
	state.HPLTPredCodGainQ7 = delta + fix.RShiftRound(state.HPLTPredCodGainQ7, 1)
	state.PrevLTPredCodGainQ7 = state.LTPredCodGainQ7

	// Combine raw + filtered gain, then sigmoid.
	gOutQ5 := fix.RShiftRound(
		fix.RShift32(state.LTPredCodGainQ7, 1)+fix.RShift32(state.HPLTPredCodGainQ7, 1), 3)
	gLimitQ15 := dsp.SigmQ15(gOutQ5 - (3 << 5))

	// Default: minimum scaling.
	state.LTPScaleIndex = 0

	roundLoss := state.PacketLossPerc

	// Only re-evaluate if this is the first frame in the payload buffer.
	if state.NFramesInPayloadBuf == 0 {
		framesPerPacket := fix.Div32By16(state.PacketSizeMs, FrameLengthMs)
		roundLoss += framesPerPacket - 1

		// C uses SKP_min_int (upper clamp only); the codebase guarantees
		// PacketSize_ms > 0 in production, keeping roundLoss ≥ 0 inside this
		// branch. Go defends against the lower bound too — a zero-init state
		// in tests would otherwise OOB the LUT.
		clamp := func(v int32) int32 {
			if v < 0 {
				return 0
			}
			if v > scaleCtrlNbThresholds-1 {
				return scaleCtrlNbThresholds - 1
			}
			return v
		}
		idx1 := clamp(roundLoss)
		idx2 := clamp(roundLoss + 1)
		thrld1 := ltpScaleThresholdsQ15[idx1]
		thrld2 := ltpScaleThresholdsQ15[idx2]

		switch {
		case gLimitQ15 > thrld1:
			state.LTPScaleIndex = 2
		case gLimitQ15 > thrld2:
			state.LTPScaleIndex = 1
		}
	}

	state.LTPScaleQ14 = int32(tables.LTPScales_table_Q14[state.LTPScaleIndex])
}
