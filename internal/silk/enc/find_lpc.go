package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
)

// FindLPCFIX — SKP_Silk_find_LPC_FIX.
//
// Run Burg LPC analysis on the LTP-residual / pre-emphasized input, then —
// when NLSF interpolation is enabled — also fit a half-frame LPC and
// search over interpolation factors {0..3} to find the lowest residual
// energy. *interpIndex receives 4 to mean "no interpolation"; otherwise
// the chosen factor.
//
// Translated from vendor/silk/src/SKP_Silk_find_LPC_FIX.c.
func FindLPCFIX(
	NLSFQ15 []int32,
	interpIndex *int32,
	prevNLSFqQ15 []int32,
	useInterpolatedNLSFs int32,
	lpcOrder int32,
	x []int16,
	subfrLength int32,
) {
	var aQ16 [silk.MaxLPCOrder]int32
	var aTmpQ16 [silk.MaxLPCOrder]int32
	var aTmpQ12 [silk.MaxLPCOrder]int16
	var NLSF0Q15 [silk.MaxLPCOrder]int32
	var S [silk.MaxLPCOrder]int16
	var lpcRes [(silk.MaxFrameLength + silk.NBSubFr*silk.MaxLPCOrder) / 2]int16

	*interpIndex = 4 // default: no interpolation

	// Full-frame Burg analysis.
	var resNrg, resNrgQ int32
	dsp.BurgModified(&resNrg, &resNrgQ, aQ16[:], x, subfrLength, silk.NBSubFr,
		fix.FixConst32(float64(silk.FindLPCCondFac), 32), lpcOrder)
	dsp.BwExpander32(aQ16[:], lpcOrder, fix.FixConst32(float64(silk.FindLPCChirp), 16))

	if useInterpolatedNLSFs == 1 {
		// Optimal LPC for the last 10 ms (half frame).
		var resTmpNrg, resTmpNrgQ int32
		halfSubfr := int32(silk.NBSubFr) >> 1
		dsp.BurgModified(&resTmpNrg, &resTmpNrgQ, aTmpQ16[:],
			x[halfSubfr*subfrLength:], subfrLength, halfSubfr,
			fix.FixConst32(float64(silk.FindLPCCondFac), 32), lpcOrder)
		dsp.BwExpander32(aTmpQ16[:], lpcOrder, fix.FixConst32(float64(silk.FindLPCChirp), 16))

		// Subtract second-half residual energy from the full-frame one
		// (stays in matching Q domain).
		shift := resTmpNrgQ - resNrgQ
		switch {
		case shift >= 0:
			if shift < 32 {
				resNrg -= fix.RShift32(resTmpNrg, shift)
			}
		default:
			resNrg = fix.RShift32(resNrg, -shift) - resTmpNrg
			resNrgQ = resTmpNrgQ
		}

		// Convert to NLSFs (the second-half AR).
		nlsf.A2NLSF(NLSFQ15, aTmpQ16[:], lpcOrder)

		// Search interp factors 3..0 for the one minimizing total energy.
		for k := int32(3); k >= 0; k-- {
			dsp.Interpolate(NLSF0Q15[:], prevNLSFqQ15, NLSFQ15, k, lpcOrder)
			nlsf.NLSF2AStable(aTmpQ12[:], NLSF0Q15[:], lpcOrder)

			// Reset filter state and run analysis filter.
			for i := range S[:lpcOrder] {
				S[i] = 0
			}
			dsp.LPCAnalysisFilter(x, aTmpQ12[:], S[:], lpcRes[:], 2*subfrLength, lpcOrder)

			resNrg0, rshift0 := dsp.SumSqrShift(lpcRes[lpcOrder:], subfrLength-lpcOrder)
			resNrg1, rshift1 := dsp.SumSqrShift(lpcRes[lpcOrder+subfrLength:], subfrLength-lpcOrder)

			// Sum the two halves with matching Q.
			var resNrgInterp int32
			var resNrgInterpQ int32
			shift := rshift0 - rshift1
			if shift >= 0 {
				resNrg1 = fix.RShift32(resNrg1, shift)
				resNrgInterpQ = -rshift0
			} else {
				resNrg0 = fix.RShift32(resNrg0, -shift)
				resNrgInterpQ = -rshift1
			}
			resNrgInterp = resNrg0 + resNrg1

			// Compare against the running best.
			isInterpLower := false
			shift = resNrgInterpQ - resNrgQ
			if shift >= 0 {
				if fix.RShift32(resNrgInterp, shift) < resNrg {
					isInterpLower = true
				}
			} else if -shift < 32 {
				if resNrgInterp < fix.RShift32(resNrg, -shift) {
					isInterpLower = true
				}
			}

			if isInterpLower {
				resNrg = resNrgInterp
				resNrgQ = resNrgInterpQ
				*interpIndex = k
			}
		}
	}

	if *interpIndex == 4 {
		// No interpolation chosen — convert the full-frame AR.
		nlsf.A2NLSF(NLSFQ15, aQ16[:], lpcOrder)
	}
}
