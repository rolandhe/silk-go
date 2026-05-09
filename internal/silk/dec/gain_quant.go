package dec

import "github.com/rolandhe/silk-go/internal/silk/dsp"

// GainsQuant — re-export of dsp.GainsQuant. Kept here for backwards
// compatibility with decoder tests; the encoder uses dsp.GainsQuant directly.
func GainsQuant(ind []int32, gainQ16 []int32, prevInd *int32, conditional int32) {
	dsp.GainsQuant(ind, gainQ16, prevInd, conditional)
}

// GainsDequant — re-export of dsp.GainsDequant.
func GainsDequant(gainQ16 []int32, ind []int32, prevInd *int32, conditional int32) {
	dsp.GainsDequant(gainQ16, ind, prevInd, conditional)
}
