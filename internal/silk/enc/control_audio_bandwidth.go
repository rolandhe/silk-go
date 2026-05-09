package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// ControlAudioBandwidth — SKP_Silk_control_audio_bandwidth.
//
// Pick the internal sampling rate (8/12/16/24 kHz) for the next frame. On
// the first call (`FsKHz == 0`) the rate is chosen from the target bitrate
// alone; afterwards a small state machine accumulates the rate margin and
// switches down when bandwidth runs short and back up when it returns,
// gated by the SWITCH_TRANSITION_FILTERING transition machinery.
//
// Returns the new internal rate in kHz.
//
// Translated from vendor/silk/src/SKP_Silk_control_audio_bandwidth.c.
func ControlAudioBandwidth(c *CommonState, targetRateBPS int32) int32 {
	fsKHz := c.FsKHz

	switch {
	case fsKHz == 0:
		// Initial pick from target bitrate.
		switch {
		case targetRateBPS >= silk.SWB2WBBitrateBPS:
			fsKHz = 24
		case targetRateBPS >= silk.WB2MBBitrateBPS:
			fsKHz = 16
		case targetRateBPS >= silk.MB2NBBitrateBPS:
			fsKHz = 12
		default:
			fsKHz = 8
		}
		fsKHz = fix.MinInt(fsKHz, c.APIFsHz/1000)
		fsKHz = fix.MinInt(fsKHz, c.MaxInternalFsKHz)

	case fsKHz*1000 > c.APIFsHz || fsKHz > c.MaxInternalFsKHz:
		// External constraints lowered — clamp.
		fsKHz = c.APIFsHz / 1000
		fsKHz = fix.MinInt(fsKHz, c.MaxInternalFsKHz)

	default:
		// State machine.
		if c.APIFsHz > 8000 {
			// Down-switch margin accumulator (saturates at 0).
			c.BitrateDiff += c.PacketSizeMs * (targetRateBPS - c.BitrateThresholdDown)
			c.BitrateDiff = fix.MinInt(c.BitrateDiff, 0)

			if c.VadFlag == silk.NoVoiceActivity {
				// Down-switch arming gate.
				switch {
				case c.SLP.TransitionFrameNo == 0 &&
					(c.BitrateDiff <= -silk.AccumBitsDiffThreshold ||
						c.SSWBDetect.WBDetected*c.FsKHz == 24):
					c.SLP.TransitionFrameNo = 1
					c.SLP.Mode = 0
				case c.SLP.TransitionFrameNo >= silk.TransitionFramesDown && c.SLP.Mode == 0:
					// Transition complete → commit to the new rate.
					c.SLP.TransitionFrameNo = 0
					c.BitrateDiff = 0
					switch c.FsKHz {
					case 24:
						fsKHz = 16
					case 16:
						fsKHz = 12
					case 12:
						fsKHz = 8
					}
				}

				// Up-switch (only fires if the API and max rates allow
				// it, and no transition is already running).
				switchUpReady := false
				switch c.FsKHz {
				case 16:
					switchUpReady = c.MaxInternalFsKHz >= 24
				case 12:
					switchUpReady = c.MaxInternalFsKHz >= 16
				case 8:
					switchUpReady = c.MaxInternalFsKHz >= 12
				}
				if c.FsKHz*1000 < c.APIFsHz &&
					targetRateBPS >= c.BitrateThresholdUp &&
					c.SSWBDetect.WBDetected*c.FsKHz < 16 &&
					switchUpReady &&
					c.SLP.TransitionFrameNo == 0 {
					c.SLP.Mode = 1
					c.BitrateDiff = 0
					switch c.FsKHz {
					case 8:
						fsKHz = 12
					case 12:
						fsKHz = 16
					case 16:
						fsKHz = 24
					}
				}
			}
		}

		// After an up-switch, drop the transition filter when speech
		// goes inactive so we don't keep spending state on it.
		if c.SLP.Mode == 1 &&
			c.SLP.TransitionFrameNo >= silk.TransitionFramesUp &&
			c.VadFlag == silk.NoVoiceActivity {
			c.SLP.TransitionFrameNo = 0
			c.SLP.InLPState[0] = 0
			c.SLP.InLPState[1] = 0
		}
	}

	return fsKHz
}
