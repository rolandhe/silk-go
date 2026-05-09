package nlsf

import (
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/lpc"
)

// MaxLPCStabilizeIterations — SKP_Silk_define.h MAX_LPC_STABILIZE_ITERATIONS.
const MaxLPCStabilizeIterations = 20

// NLSF2AStable — SKP_Silk_NLSF2A_stable.
//
// Convert NLSF (Q15) to AR coefficients (Q12), then apply progressively more
// bandwidth expansion until the resulting LPC is stable (all poles inside
// the unit circle) or MaxLPCStabilizeIterations have been spent. If still
// unstable, zero the output.
//
// Translated from vendor/silk/src/SKP_Silk_NLSF2A_stable.c.
func NLSF2AStable(arQ12 []int16, nlsf []int32, lpcOrder int32) {
	NLSF2A(arQ12, nlsf, lpcOrder)

	var i int32
	var invGainQ30 int32
	for i = 0; i < MaxLPCStabilizeIterations; i++ {
		if lpc.LPCInversePredGain(&invGainQ30, arQ12, lpcOrder) == 1 {
			// Apply bandwidth expansion. 10_Q16 ≈ 0.00015.
			dsp.BwExpander(arQ12, lpcOrder, 65536-fix.SmulBB(10+i, i))
		} else {
			break
		}
	}

	if i == MaxLPCStabilizeIterations {
		for i = 0; i < lpcOrder; i++ {
			arQ12[i] = 0
		}
	}
}
