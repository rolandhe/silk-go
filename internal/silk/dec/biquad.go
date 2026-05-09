package dec

import "github.com/rolandhe/silk-go/internal/silk/dsp"

// Biquad — re-export of dsp.Biquad. Kept here for backwards compatibility
// with decoder code and tests that called Biquad before it migrated to the
// dsp package (the encoder needs it too).
func Biquad(in []int16, B []int16, A []int16, S []int32, out []int16, length int32) {
	dsp.Biquad(in, B, A, S, out, length)
}

// BiquadAlt — re-export of dsp.BiquadAlt.
func BiquadAlt(in []int16, BQ28 []int32, AQ28 []int32, S []int32, out []int16, length int32) {
	dsp.BiquadAlt(in, BQ28, AQ28, S, out, length)
}
