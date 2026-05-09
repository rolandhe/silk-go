package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Interpolate — SKP_Silk_interpolate.
//
// xi[i] = x0[i] + ((x1[i] - x0[i]) * ifactQ2) >> 2.
// ifactQ2 ∈ [0, 4]; d ≤ MaxLPCOrder (16).
//
// Translated from vendor/silk/src/SKP_Silk_interpolate.c.
func Interpolate(xi, x0, x1 []int32, ifactQ2 int32, d int32) {
	for i := int32(0); i < d; i++ {
		xi[i] = x0[i] + fix.RShift32(fix.Mul(x1[i]-x0[i], ifactQ2), 2)
	}
}
