// Package dec implements the SILK decoder pipeline ported from
// vendor/silk/src/SKP_Silk_decode*.c plus a few decoder-side utilities
// (shell coder, biquad, MA, gain dequant, etc.).
package dec

import "github.com/rolandhe/silk-go/internal/silk/dsp"

// Interpolate — re-export of dsp.Interpolate. Kept here for backwards
// compatibility with decoder tests; the encoder uses dsp.Interpolate directly.
func Interpolate(xi, x0, x1 []int32, ifactQ2 int32, d int32) {
	dsp.Interpolate(xi, x0, x1, ifactQ2, d)
}
