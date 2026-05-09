package dec

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// pitchEstMinLagMs mirrors PITCH_EST_MIN_LAG_MS (= 2) from
// vendor/silk/src/SKP_Silk_pitch_est_defines.h.
const pitchEstMinLagMs = 2

// pitchEstNBSubFr — PITCH_EST_NB_SUBFR (= 4).
const pitchEstNBSubFr = 4

// DecodePitch — SKP_Silk_decode_pitch.
//
// Reconstruct 4 per-subframe pitch lags from a single lag index plus a
// contour-codebook index. The 8 kHz path uses the smaller stage-2 codebook;
// every other rate uses stage-3.
//
// Translated from vendor/silk/src/SKP_Silk_decode_pitch.c.
func DecodePitch(lagIndex, contourIndex int32, pitchLags []int32, FsKHz int32) {
	minLag := fix.SmulBB(pitchEstMinLagMs, FsKHz)
	lag := minLag + lagIndex
	if FsKHz == 8 {
		for i := int32(0); i < pitchEstNBSubFr; i++ {
			pitchLags[i] = lag + int32(tables.CB_lags_stage2[i][contourIndex])
		}
	} else {
		for i := int32(0); i < pitchEstNBSubFr; i++ {
			pitchLags[i] = lag + int32(tables.CB_lags_stage3[i][contourIndex])
		}
	}
}
