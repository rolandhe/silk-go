package nlsf

import (
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

const stabilizeMaxLoops = 20

// Stabilize — SKP_Silk_NLSF_stabilize.
//
// Move NLSFs apart to satisfy a minimum delta and stay inside the
// (NDeltaMin_Q15[0], 1<<15 - NDeltaMin_Q15[L]) range. Fall back to a sort +
// monotone clamp if the iterative pass doesn't converge in MAX_LOOPS.
//
// nlsfQ15 is length L (in/out). nDeltaMinQ15 is length L+1 (input only).
//
// Translated from vendor/silk/src/SKP_Silk_NLSF_stabilize.c.
func Stabilize(nlsfQ15 []int32, nDeltaMinQ15 []int32, L int32) {
	var loops int32
	for loops = 0; loops < stabilizeMaxLoops; loops++ {
		// Find smallest distance.
		minDiff := nlsfQ15[0] - nDeltaMinQ15[0]
		I := int32(0)
		for i := int32(1); i <= L-1; i++ {
			diff := nlsfQ15[i] - (nlsfQ15[i-1] + nDeltaMinQ15[i])
			if diff < minDiff {
				minDiff = diff
				I = i
			}
		}
		// Last element.
		diff := (int32(1) << 15) - (nlsfQ15[L-1] + nDeltaMinQ15[L])
		if diff < minDiff {
			minDiff = diff
			I = L
		}

		if minDiff >= 0 {
			return
		}

		switch {
		case I == 0:
			nlsfQ15[0] = nDeltaMinQ15[0]
		case I == L:
			nlsfQ15[L-1] = (int32(1) << 15) - nDeltaMinQ15[L]
		default:
			// Lower extreme for current center frequency.
			minCenter := int32(0)
			for k := int32(0); k < I; k++ {
				minCenter += nDeltaMinQ15[k]
			}
			minCenter += fix.RShift32(nDeltaMinQ15[I], 1)

			// Upper extreme.
			maxCenter := int32(1) << 15
			for k := L; k > I; k-- {
				maxCenter -= nDeltaMinQ15[k]
			}
			maxCenter -= nDeltaMinQ15[I] - fix.RShift32(nDeltaMinQ15[I], 1)

			// Move apart, sorted by value, keeping the same center frequency.
			centerFreq := fix.Limit(
				fix.RShiftRound(nlsfQ15[I-1]+nlsfQ15[I], 1),
				minCenter, maxCenter)
			nlsfQ15[I-1] = centerFreq - fix.RShift32(nDeltaMinQ15[I], 1)
			nlsfQ15[I] = nlsfQ15[I-1] + nDeltaMinQ15[I]
		}
	}

	// Fallback: insertion sort + monotone clamp.
	if loops == stabilizeMaxLoops {
		dsp.InsertionSortIncreasingAllValues(nlsfQ15, L)

		nlsfQ15[0] = fix.MaxInt(nlsfQ15[0], nDeltaMinQ15[0])
		for i := int32(1); i < L; i++ {
			nlsfQ15[i] = fix.MaxInt(nlsfQ15[i], nlsfQ15[i-1]+nDeltaMinQ15[i])
		}
		nlsfQ15[L-1] = fix.MinInt(nlsfQ15[L-1], (int32(1)<<15)-nDeltaMinQ15[L])
		for i := L - 2; i >= 0; i-- {
			nlsfQ15[i] = fix.MinInt(nlsfQ15[i], nlsfQ15[i+1]-nDeltaMinQ15[i+1])
		}
	}
}
