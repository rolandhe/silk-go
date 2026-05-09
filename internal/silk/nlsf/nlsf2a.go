package nlsf

import (
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// MaxOrderLPC — SKP_Silk_MAX_ORDER_LPC, kept as a local constant to avoid
// pulling the lpc package (cyclic with k2a / inv_pred_gain).
const MaxOrderLPC = 16

// findPoly — SKP_Silk_NLSF2A_find_poly.
//
// Generate the even/odd intermediate polynomial in Q20 from the interleaved
// 2*cos(LSF) values (also Q20). dd = polynomial order (= filter order / 2).
func findPoly(out []int32, cLSF []int32, dd int32) {
	out[0] = int32(1) << 20
	out[1] = -cLSF[0]
	for k := int32(1); k < dd; k++ {
		ftmp := cLSF[2*k] // Q20
		out[k+1] = fix.LShift32(out[k-1], 1) - int32(fix.RShiftRound64(fix.Smull(ftmp, out[k]), 20))
		for n := k; n > 1; n-- {
			out[n] += out[n-2] - int32(fix.RShiftRound64(fix.Smull(ftmp, out[n-1]), 20))
		}
		out[1] -= ftmp
	}
}

// NLSF2A — SKP_Silk_NLSF2A.
//
// Convert normalized line spectral frequencies (Q15) to monic whitening
// filter coefficients (Q12). d (filter order) must be even and <=16.
//
// Translated from vendor/silk/src/SKP_Silk_NLSF2A.c.
func NLSF2A(a []int16, nlsf []int32, d int32) {
	var cosLSFQ20 [MaxOrderLPC]int32
	var P, Q [MaxOrderLPC/2 + 1]int32

	// Convert NLSFs to 2*cos(NLSF), piecewise-linear via the cos table.
	for k := int32(0); k < d; k++ {
		fInt := fix.RShift32(nlsf[k], 15-7)         // 0..127
		fFrac := nlsf[k] - fix.LShift32(fInt, 15-7) // 0..255
		cosVal := tables.LSFCosTab_FIX_Q12[fInt]    // Q12
		delta := tables.LSFCosTab_FIX_Q12[fInt+1] - cosVal
		cosLSFQ20[k] = fix.LShift32(cosVal, 8) + fix.Mul(delta, fFrac) // Q20
	}

	dd := fix.RShift32(d, 1)
	findPoly(P[:], cosLSFQ20[0:], dd)
	findPoly(Q[:], cosLSFQ20[1:], dd)

	// Convert even/odd polynomials to int32 Q12 filter coefficients.
	var aInt32 [MaxOrderLPC]int32
	for k := int32(0); k < dd; k++ {
		Ptmp := P[k+1] + P[k]
		Qtmp := Q[k+1] - Q[k]
		aInt32[k] = -fix.RShiftRound(Ptmp+Qtmp, 9) // Q20 → Q12
		aInt32[d-k-1] = fix.RShiftRound(Qtmp-Ptmp, 9)
	}

	// Limit max absolute value of the prediction coefficients.
	for i := 0; i < 10; i++ {
		var maxabs int32
		var idx int32
		for k := int32(0); k < d; k++ {
			absval := fix.Abs32(aInt32[k])
			if absval > maxabs {
				maxabs = absval
				idx = k
			}
		}
		if maxabs > 0x7FFF {
			// Reduce magnitude. The literal 98369 is from the C source:
			//   ( SKP_int32_MAX / ( 65470 >> 2 ) ) + SKP_int16_MAX = 98369
			if maxabs > 98369 {
				maxabs = 98369
			}
			scQ16 := int32(65470) - fix.Div32(
				fix.Mul(65470>>2, maxabs-0x7FFF),
				fix.RShift32(fix.Mul(maxabs, idx+1), 2),
			)
			dsp.BwExpander32(aInt32[:], d, scQ16)
		} else {
			break
		}
	}

	// Final saturate-pack into int16 Q12 (the C SKP_assert(0) is hit only on
	// pathological inputs that never converged; we mirror the saturation).
	for k := int32(0); k < d; k++ {
		v := aInt32[k]
		if v > 0x7FFF {
			v = 0x7FFF
		} else if v < -0x8000 {
			v = -0x8000
		}
		a[k] = int16(v)
	}
}
