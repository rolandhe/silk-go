package ltp

import "github.com/rolandhe/silk-go/internal/silk/tables"

// QuantGains — SKP_Silk_quant_LTP_gains_FIX.
//
// Quantize the 4×LTP_ORDER LTP gain matrix. For each of the 3 codebooks
// (with different rate/distortion tradeoffs), per-subframe nearest-
// -neighbour search via VQWMatECFix; pick the codebook with the lowest
// total weighted RD. In low-complexity mode, break early once a codebook
// is "good enough" (compared to LTP_gain_middle_avg_RD_Q14).
//
//	BQ14            — IN: unquantized LTP gains [NB_SUBFR × LTP_ORDER]
//	                  OUT: quantized gains, replaced from chosen codebook
//	cbkIndex        — output: per-subframe codebook index [NB_SUBFR]
//	periodicityIndex— output: which of the 3 codebooks won
//	WQ18            — error weights, [NB_SUBFR × LTP_ORDER × LTP_ORDER]
//	muQ8            — rate/distortion tradeoff
//	lowComplexity   — non-zero to enable the early-break shortcut
//
// Translated from vendor/silk/src/SKP_Silk_quant_LTP_gains_FIX.c.
func QuantGains(BQ14 []int16, cbkIndex []int32, periodicityIndex *int32,
	WQ18 []int32, muQ8 int32, lowComplexity int32) {

	var tempIdx [NBSubFr]int32
	minRateDist := int32(0x7FFFFFFF)
	*periodicityIndex = 0

	for k := int32(0); k < 3; k++ {
		clPtr := tables.LTPGainBITSQ6Ptrs[k]
		cbkPtr := tables.LTPVQPtrsQ14[k]
		cbkSize := tables.LTP_vq_sizes[k]

		var rateDist int32
		for j := int32(0); j < NBSubFr; j++ {
			var subRD int32
			VQWMatECFix(
				&tempIdx[j],
				&subRD,
				BQ14[j*LTPOrder:j*LTPOrder+LTPOrder],
				WQ18[j*LTPOrder*LTPOrder:(j+1)*LTPOrder*LTPOrder],
				cbkPtr,
				clPtr,
				muQ8,
				cbkSize,
			)
			// SKP_ADD_POS_SAT32 semantics — caps at int32_max if the sum's
			// sign bit flips.
			if rd := rateDist + subRD; uint32(rd)&0x80000000 != 0 {
				rateDist = 0x7FFFFFFF
			} else {
				rateDist = rd
			}
		}
		if rateDist > 0x7FFFFFFE {
			rateDist = 0x7FFFFFFE
		}

		if rateDist < minRateDist {
			minRateDist = rateDist
			copy(cbkIndex, tempIdx[:])
			*periodicityIndex = k
		}

		if lowComplexity != 0 && rateDist < tables.LTP_gain_middle_avg_RD_Q14 {
			break
		}
	}

	// Replace BQ14 with the chosen codebook entries.
	cbkPtr := tables.LTPVQPtrsQ14[*periodicityIndex]
	for j := int32(0); j < NBSubFr; j++ {
		row := cbkIndex[j] * LTPOrder
		for k := int32(0); k < LTPOrder; k++ {
			BQ14[j*LTPOrder+k] = cbkPtr[row+k]
		}
	}
}
