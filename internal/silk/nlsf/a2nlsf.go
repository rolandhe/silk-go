package nlsf

import (
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// A2NLSF constants — mirror SKP_Silk_A2NLSF.c. We don't enable the
// OVERSAMPLE_COSINE_TABLE branch (the C source ships with it disabled).
const (
	binDivStepsA2NLSF   = 3
	qPoly               = 16
	maxIterationsA2NLSF = 30
	lsfCosTabSize       = 128 // = LSF_COS_TAB_SZ_FIX
)

// a2nlsfTransPoly — SKP_Silk_A2NLSF_trans_poly.
//
// Transform the polynomial in `p` from the cos(n*f) basis to the cos(f)^n
// basis. dd = filter order / 2.
func a2nlsfTransPoly(p []int32, dd int32) {
	for k := int32(2); k <= dd; k++ {
		for n := dd; n > k; n-- {
			p[n-2] -= p[n]
		}
		p[k-2] -= fix.LShift32(p[k], 1)
	}
}

// a2nlsfEvalPoly — SKP_Silk_A2NLSF_eval_poly.
//
// Horner-style evaluation of a QPoly polynomial at x (Q12), returning a
// QPoly result. Uses SmlaWW (= a + ((b*c) >> 16)).
func a2nlsfEvalPoly(p []int32, x int32, dd int32) int32 {
	y32 := p[dd]
	xQ16 := fix.LShift32(x, 4)
	for n := dd - 1; n >= 0; n-- {
		y32 = fix.SmlaWW(p[n], y32, xQ16)
	}
	return y32
}

// a2nlsfInit — SKP_Silk_A2NLSF_init.
//
// Build the even (P) and odd (Q) polynomials from the AR coefficients
// (Q16), divide out the always-present roots z=1 (Q) and z=-1 (P), and
// transform to the cos(f)^n basis.
func a2nlsfInit(aQ16 []int32, P, Q []int32, dd int32) {
	// QPoly == 16 in the C source — this branch is bit-identical.
	P[dd] = int32(1) << qPoly
	Q[dd] = int32(1) << qPoly
	for k := int32(0); k < dd; k++ {
		P[k] = -aQ16[dd-k-1] - aQ16[dd+k]
		Q[k] = -aQ16[dd-k-1] + aQ16[dd+k]
	}

	// Divide out z=1 (in Q) and z=-1 (in P).
	for k := dd; k > 0; k-- {
		P[k-1] -= P[k]
		Q[k-1] += Q[k]
	}

	a2nlsfTransPoly(P, dd)
	a2nlsfTransPoly(Q, dd)
}

// A2NLSF — SKP_Silk_A2NLSF.
//
// Compute Normalized Line Spectral Frequencies (Q15) from monic whitening
// filter coefficients (Q16). Modifies aQ16 in place if bandwidth expansion
// is needed. d must be even.
//
// Translated from vendor/silk/src/SKP_Silk_A2NLSF.c.
func A2NLSF(NLSF []int32, aQ16 []int32, d int32) {
	var P, Q [MaxOrderLPC/2 + 1]int32

	dd := fix.RShift32(d, 1)
	a2nlsfInit(aQ16, P[:], Q[:], dd)

	// Pointer to current polynomial: alternates P / Q.
	p := P[:]

	xlo := tables.LSFCosTab_FIX_Q12[0] // Q12
	ylo := a2nlsfEvalPoly(p, xlo, dd)

	var rootIx int32
	if ylo < 0 {
		NLSF[0] = 0
		p = Q[:]
		ylo = a2nlsfEvalPoly(p, xlo, dd)
		rootIx = 1
	}
	k := int32(1)
	i := int32(0)
	for {
		// Evaluate polynomial at the next cos-table entry.
		xhi := tables.LSFCosTab_FIX_Q12[k] // Q12
		yhi := a2nlsfEvalPoly(p, xhi, dd)

		// Detect zero crossing.
		if (ylo <= 0 && yhi >= 0) || (ylo >= 0 && yhi <= 0) {
			// Binary division.
			ffrac := int32(-256)
			for m := int32(0); m < binDivStepsA2NLSF; m++ {
				xmid := fix.RShiftRound(xlo+xhi, 1)
				ymid := a2nlsfEvalPoly(p, xmid, dd)
				if (ylo <= 0 && ymid >= 0) || (ylo >= 0 && ymid <= 0) {
					xhi = xmid
					yhi = ymid
				} else {
					xlo = xmid
					ylo = ymid
					ffrac = fix.AddRShift32(ffrac, 128, m)
				}
			}

			// Interpolate.
			if fix.Abs32(ylo) < 65536 {
				den := ylo - yhi
				nom := fix.LShift32(ylo, 8-binDivStepsA2NLSF) + fix.RShift32(den, 1)
				if den != 0 {
					ffrac += fix.Div32(nom, den)
				}
			} else {
				ffrac += fix.Div32(ylo, fix.RShift32(ylo-yhi, 8-binDivStepsA2NLSF))
			}
			v := fix.LShift32(k, 8) + ffrac
			if v > 0x7FFF {
				v = 0x7FFF
			}
			NLSF[rootIx] = v

			rootIx++
			if rootIx >= d {
				return
			}
			// Alternate polynomial.
			if rootIx&1 != 0 {
				p = Q[:]
			} else {
				p = P[:]
			}

			xlo = tables.LSFCosTab_FIX_Q12[k-1]
			// Mirror the C: ylo is forced to ±1 in Q12 depending on rootIx.
			ylo = fix.LShift32(1-(rootIx&2), 12)
		} else {
			k++
			xlo = xhi
			ylo = yhi

			if k > lsfCosTabSize {
				i++
				if i > maxIterationsA2NLSF {
					// Set NLSFs to a white spectrum and exit.
					NLSF[0] = fix.Div32By16(int32(1)<<15, d+1)
					for kk := int32(1); kk < d; kk++ {
						NLSF[kk] = fix.SmulBB(kk+1, NLSF[0])
					}
					return
				}

				// Apply progressively more bandwidth expansion and retry.
				dsp.BwExpander32(aQ16, d, 65536-fix.SmulBB(10+i, i))

				a2nlsfInit(aQ16, P[:], Q[:], dd)
				p = P[:]
				xlo = tables.LSFCosTab_FIX_Q12[0]
				ylo = a2nlsfEvalPoly(p, xlo, dd)
				if ylo < 0 {
					NLSF[0] = 0
					p = Q[:]
					ylo = a2nlsfEvalPoly(p, xlo, dd)
					rootIx = 1
				} else {
					rootIx = 0
				}
				k = 1
			}
		}
	}
}
