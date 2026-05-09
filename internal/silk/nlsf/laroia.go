// Package nlsf ports NLSF helpers from vendor/silk/src/SKP_Silk_NLSF*.c
// (Normalized Line Spectral Frequencies — used by SILK's LPC quantizer).
package nlsf

import "github.com/rolandhe/silk-go/internal/silk/fix"

const (
	laroiaQOut      = 6
	laroiaMinNDelta = 3
)

// VQWeightsLaroia — SKP_Silk_NLSF_VQ_weights_laroia.
//
// Laroia low-complexity NLSF weights. D must be even and > 0.
// Output is Q6 weights, capped at int16_max.
//
// Translated from vendor/silk/src/SKP_Silk_NLSF_VQ_weights_laroia.c.
func VQWeightsLaroia(weightsQ6 []int32, nlsfQ15 []int32, D int32) {
	if D <= 0 || D&1 != 0 {
		// SKP_assert in C; mirror with a no-op rather than panic to match
		// release-build behavior.
		return
	}

	// First value.
	tmp1 := fix.MaxInt(nlsfQ15[0], laroiaMinNDelta)
	tmp1 = fix.Div32By16(int32(1)<<(15+laroiaQOut), tmp1)
	tmp2 := fix.MaxInt(nlsfQ15[1]-nlsfQ15[0], laroiaMinNDelta)
	tmp2 = fix.Div32By16(int32(1)<<(15+laroiaQOut), tmp2)
	weightsQ6[0] = fix.MinInt(tmp1+tmp2, 0x7FFF)

	// Main loop: indices 1..D-2 step 2.
	for k := int32(1); k < D-1; k += 2 {
		tmp1 = fix.MaxInt(nlsfQ15[k+1]-nlsfQ15[k], laroiaMinNDelta)
		tmp1 = fix.Div32By16(int32(1)<<(15+laroiaQOut), tmp1)
		weightsQ6[k] = fix.MinInt(tmp1+tmp2, 0x7FFF)

		tmp2 = fix.MaxInt(nlsfQ15[k+2]-nlsfQ15[k+1], laroiaMinNDelta)
		tmp2 = fix.Div32By16(int32(1)<<(15+laroiaQOut), tmp2)
		weightsQ6[k+1] = fix.MinInt(tmp1+tmp2, 0x7FFF)
	}

	// Last value.
	tmp1 = fix.MaxInt((int32(1)<<15)-nlsfQ15[D-1], laroiaMinNDelta)
	tmp1 = fix.Div32By16(int32(1)<<(15+laroiaQOut), tmp1)
	weightsQ6[D-1] = fix.MinInt(tmp1+tmp2, 0x7FFF)
}
