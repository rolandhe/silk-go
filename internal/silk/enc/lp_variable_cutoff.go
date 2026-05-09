package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// lpInterpolateFilterTaps — SKP_Silk_LP_interpolate_filter_taps.
//
// Build the active set of biquad coefficients by linearly interpolating
// between adjacent rows of the elliptic-LP design table. Q16 fits in 16
// bits in two of the three branches, so the C source uses SmlaWB; the
// edge cases around fac == (1<<15) and fac > (1<<16) - SAT16 use either an
// average or the symmetric SmlaWB.
func lpInterpolateFilterTaps(BQ28, AQ28 []int32, ind, facQ16 int32) {
	if ind < silk.TransitionIntNum-1 {
		switch {
		case facQ16 > 0 && facQ16 == fix.Sat16(facQ16):
			for nb := 0; nb < silk.TransitionNB; nb++ {
				BQ28[nb] = fix.SmlaWB(
					tables.Transition_LP_B_Q28[ind][nb],
					tables.Transition_LP_B_Q28[ind+1][nb]-tables.Transition_LP_B_Q28[ind][nb],
					facQ16)
			}
			for na := 0; na < silk.TransitionNA; na++ {
				AQ28[na] = fix.SmlaWB(
					tables.Transition_LP_A_Q28[ind][na],
					tables.Transition_LP_A_Q28[ind+1][na]-tables.Transition_LP_A_Q28[ind][na],
					facQ16)
			}
		case facQ16 == (1 << 15):
			// Midpoint — neither fac nor (1<<16)-fac fits in int16, so
			// fall back to a plain average.
			for nb := 0; nb < silk.TransitionNB; nb++ {
				BQ28[nb] = fix.RShift32(
					tables.Transition_LP_B_Q28[ind][nb]+tables.Transition_LP_B_Q28[ind+1][nb], 1)
			}
			for na := 0; na < silk.TransitionNA; na++ {
				AQ28[na] = fix.RShift32(
					tables.Transition_LP_A_Q28[ind][na]+tables.Transition_LP_A_Q28[ind+1][na], 1)
			}
		case facQ16 > 0:
			// (1<<16) - facQ16 fits in int16 — interpolate from the
			// upper neighbor backwards.
			fac := (1 << 16) - facQ16
			for nb := 0; nb < silk.TransitionNB; nb++ {
				BQ28[nb] = fix.SmlaWB(
					tables.Transition_LP_B_Q28[ind+1][nb],
					tables.Transition_LP_B_Q28[ind][nb]-tables.Transition_LP_B_Q28[ind+1][nb],
					fac)
			}
			for na := 0; na < silk.TransitionNA; na++ {
				AQ28[na] = fix.SmlaWB(
					tables.Transition_LP_A_Q28[ind+1][na],
					tables.Transition_LP_A_Q28[ind][na]-tables.Transition_LP_A_Q28[ind+1][na],
					fac)
			}
		default:
			copy(BQ28[:silk.TransitionNB], tables.Transition_LP_B_Q28[ind][:])
			copy(AQ28[:silk.TransitionNA], tables.Transition_LP_A_Q28[ind][:])
		}
	} else {
		copy(BQ28[:silk.TransitionNB], tables.Transition_LP_B_Q28[silk.TransitionIntNum-1][:])
		copy(AQ28[:silk.TransitionNA], tables.Transition_LP_A_Q28[silk.TransitionIntNum-1][:])
	}
}

// LPVariableCutoff — SKP_Silk_LP_variable_cutoff.
//
// Variable-cutoff low-pass filter used during fs-switching transitions.
// Set TransitionFrameNo > 0 to start a transition (mode = 0 to ramp down,
// mode = 1 to ramp up); set TransitionFrameNo = 0 to deactivate. When
// inactive the input is copied straight to the output.
//
// Translated from vendor/silk/src/SKP_Silk_LP_variable_cutoff.c.
func LPVariableCutoff(psLP *LPState, out, in []int16, frameLength int32) {
	var BQ28 [silk.TransitionNB]int32
	var AQ28 [silk.TransitionNA]int32
	var facQ16 int32
	var ind int32

	if psLP.TransitionFrameNo > 0 {
		switch psLP.Mode {
		case 0:
			if psLP.TransitionFrameNo < silk.TransitionFramesDown {
				if silk.TransitionIntStepsDn == 32 {
					facQ16 = fix.LShift32(psLP.TransitionFrameNo, 16-5)
				} else {
					facQ16 = fix.LShift32(psLP.TransitionFrameNo, 16) / silk.TransitionIntStepsDn
				}
				ind = fix.RShift32(facQ16, 16)
				facQ16 -= fix.LShift32(ind, 16)
				lpInterpolateFilterTaps(BQ28[:], AQ28[:], ind, facQ16)
				psLP.TransitionFrameNo++
			} else {
				lpInterpolateFilterTaps(BQ28[:], AQ28[:], silk.TransitionIntNum-1, 0)
			}
		case 1:
			if psLP.TransitionFrameNo < silk.TransitionFramesUp {
				if silk.TransitionIntStepsUp == 64 {
					facQ16 = fix.LShift32(silk.TransitionFramesUp-psLP.TransitionFrameNo, 16-6)
				} else {
					facQ16 = fix.LShift32(silk.TransitionFramesUp-psLP.TransitionFrameNo, 16) / silk.TransitionIntStepsUp
				}
				ind = fix.RShift32(facQ16, 16)
				facQ16 -= fix.LShift32(ind, 16)
				lpInterpolateFilterTaps(BQ28[:], AQ28[:], ind, facQ16)
				psLP.TransitionFrameNo++
			} else {
				lpInterpolateFilterTaps(BQ28[:], AQ28[:], 0, 0)
			}
		}
	}

	if psLP.TransitionFrameNo > 0 {
		dsp.BiquadAlt(in, BQ28[:], AQ28[:], psLP.InLPState[:], out, frameLength)
	} else {
		copy(out[:frameLength], in[:frameLength])
	}
}
