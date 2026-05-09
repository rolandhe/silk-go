package dec

import "github.com/rolandhe/silk-go/internal/silk/dsp"

// MAPrediction — re-export of dsp.MAPrediction.
func MAPrediction(in []int16, B []int16, S []int32, out []int16, length int32, order int32) {
	dsp.MAPrediction(in, B, S, out, length, order)
}

// LPCAnalysisFilter — re-export of dsp.LPCAnalysisFilter.
func LPCAnalysisFilter(in []int16, B []int16, S []int16, out []int16, length int32, order int32) {
	dsp.LPCAnalysisFilter(in, B, S, out, length, order)
}
