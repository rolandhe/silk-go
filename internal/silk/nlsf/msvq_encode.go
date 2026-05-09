package nlsf

import (
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// MSVQ encode constants — mirror SKP_Silk_define.h. We always build with
// LOW_COMPLEXITY_ONLY=0 and NLSF_MSVQ_FLUCTUATION_REDUCTION=1 (the C source
// defaults).
const (
	maxSurvivors      = 16  // MAX_NLSF_MSVQ_SURVIVORS
	nlsfMaxCBStages   = 10  // NLSF_MSVQ_MAX_CB_STAGES
	maxLpcOrderMSVQ   = 16  // MAX_LPC_ORDER
	treeSearchMaxVecs = 256 // NLSF_MSVQ_TREE_SEARCH_MAX_VECTORS_EVALUATED = 16*16

	// SKP_FIX_CONST(NLSF_MSVQ_SURV_MAX_REL_RD, 16) = round(0.1 * 65536) = 6554.
	survMaxRelRDQ16 = 6554
)

// MSVQEncode — SKP_Silk_NLSF_MSVQ_encode_FIX.
//
// Tree-search encoder for the multi-stage NLSF VQ. At each stage we keep
// at most `survivors` candidate paths through the codebook, ranked by
// μ·rate + weighted distortion. Optionally a fluctuation-reduction pass
// re-ranks the survivors by their distance to the previously-quantized
// NLSF vector.
//
// Inputs:
//
//	indices              — output codebook path, length nStages
//	nlsfQ15              — input NLSF (Q15); overwritten with the quantized vector
//	cb                   — codebook
//	nlsfQPrev            — previously-quantized NLSF for fluc reduction
//	wQ6                  — Laroia weights (Q6)
//	muQ15                — rate weight (Q15) for the rate-distortion sort
//	muFlucRedQ16         — fluctuation-reduction weight (Q16)
//	survivors            — max survivors per stage (≤ MaxNLSFMSVQSurvivors)
//	lpcOrder             — LPC order (10 or 16)
//	deactivateFlucRed    — when 1, skip the fluctuation reduction pass
//
// Translated from vendor/silk/src/SKP_Silk_NLSF_MSVQ_encode_FIX.c.
func MSVQEncode(
	indices []int32,
	nlsfQ15 []int32,
	cb *CBStruct,
	nlsfQPrev []int32,
	wQ6 []int32,
	muQ15 int32,
	muFlucRedQ16 int32,
	survivors int32,
	lpcOrder int32,
	deactivateFlucRed int32,
) {
	rateDistQ20 := make([]int32, treeSearchMaxVecs)
	tempIndices := make([]int32, maxSurvivors)
	rateQ5 := make([]int32, maxSurvivors)
	rateNewQ5 := make([]int32, maxSurvivors)
	resQ15 := make([]int32, maxSurvivors*maxLpcOrderMSVQ)
	resNewQ15 := make([]int32, maxSurvivors*maxLpcOrderMSVQ)
	path := make([]int32, maxSurvivors*nlsfMaxCBStages)
	pathNew := make([]int32, maxSurvivors*nlsfMaxCBStages)

	// Initial residual = input NLSF.
	for i := int32(0); i < lpcOrder; i++ {
		resQ15[i] = nlsfQ15[i]
	}
	prevSurvivors := int32(1)
	minSurvivors := survivors / 2
	curSurvivors := int32(0)

	for s := int32(0); s < cb.NStages; s++ {
		stage := &cb.CBStages[s]
		curSurvivors = fix.Min32(survivors, prevSurvivors*stage.NVectors)

		// Compute rate-distortion for all (prevSurvivors × stage.NVectors) candidates.
		VQRateDistortion(rateDistQ20[:prevSurvivors*stage.NVectors],
			stage, resQ15[:prevSurvivors*lpcOrder], wQ6, rateQ5[:prevSurvivors],
			muQ15, prevSurvivors, lpcOrder)

		// Sort top curSurvivors candidates to the front of rateDistQ20,
		// recording original positions in tempIndices.
		dsp.InsertionSortIncreasing(rateDistQ20, tempIndices,
			prevSurvivors*stage.NVectors, curSurvivors)

		// Discard survivors with RD too far above the best.
		if rateDistQ20[0] < int32(0x7FFFFFFF/maxSurvivors) {
			thresh := fix.SmlaWB(rateDistQ20[0],
				fix.Mul(survivors, rateDistQ20[0]), survMaxRelRDQ16)
			for rateDistQ20[curSurvivors-1] > thresh && curSurvivors > minSurvivors {
				curSurvivors--
			}
		}

		// Update residual / rate / path matrices for the curSurvivors winners.
		for k := int32(0); k < curSurvivors; k++ {
			var inputIdx, cbIdx int32
			if s > 0 {
				inputIdx = fix.Div32By16(tempIndices[k], stage.NVectors)
				cbIdx = tempIndices[k] - inputIdx*stage.NVectors
			} else {
				cbIdx = tempIndices[k]
			}

			// New residual = old residual − codebook row.
			resOff := inputIdx * lpcOrder
			cbOff := cbIdx * lpcOrder
			newOff := k * lpcOrder
			for i := int32(0); i < lpcOrder; i++ {
				resNewQ15[newOff+i] = resQ15[resOff+i] - int32(stage.CBNLSFQ15[cbOff+i])
			}

			// Accumulated rate.
			rateNewQ5[k] = rateQ5[inputIdx] + int32(stage.RatesQ5[cbIdx])

			// Path: copy prefix from parent, append cbIdx at position s.
			pathOff := inputIdx * cb.NStages
			pathNewOff := k * cb.NStages
			for i := int32(0); i < s; i++ {
				pathNew[pathNewOff+i] = path[pathOff+i]
			}
			pathNew[pathNewOff+s] = cbIdx
		}

		if s < cb.NStages-1 {
			copy(resQ15[:curSurvivors*lpcOrder], resNewQ15[:curSurvivors*lpcOrder])
			copy(rateQ5[:curSurvivors], rateNewQ5[:curSurvivors])
			copy(path[:curSurvivors*cb.NStages], pathNew[:curSurvivors*cb.NStages])
		}

		prevSurvivors = curSurvivors
	}

	// Pick best survivor. With fluctuation reduction we re-rank using the
	// distance to the previously-quantized NLSF.
	bestIndex := int32(0)
	if deactivateFlucRed != 1 {
		bestRateDist := int32(0x7FFFFFFF)
		tmpNLSF := make([]int32, lpcOrder)
		for i := int32(0); i < curSurvivors; i++ {
			MSVQDecode(tmpNLSF, cb,
				pathNew[i*cb.NStages:(i+1)*cb.NStages], lpcOrder)

			// Weighted squared difference to previous quantized NLSF.
			var wsseQ20 int32
			for m := int32(0); m < lpcOrder; m++ {
				se := tmpNLSF[m] - nlsfQPrev[m]
				wsseQ20 = fix.SmlaWB(wsseQ20, fix.SmulBB(se, se), wQ6[m])
			}
			combined := fix.AddPosSat32(rateDistQ20[i],
				fix.SmulWB(wsseQ20, muFlucRedQ16))
			if combined < bestRateDist {
				bestRateDist = combined
				bestIndex = i
			}
		}
	}

	// Copy best path into the output.
	for i := int32(0); i < cb.NStages; i++ {
		indices[i] = pathNew[bestIndex*cb.NStages+i]
	}

	// Decode + stabilize the chosen path. nlsfQ15 returns the quantized vector.
	MSVQDecode(nlsfQ15, cb, indices, lpcOrder)
}
