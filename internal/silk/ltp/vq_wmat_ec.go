// Package ltp ports the Long-Term Prediction (LTP) VQ helpers from
// vendor/silk/src/SKP_Silk_VQ_nearest_neighbor_FIX.c (poorly named — it
// actually holds VQ_WMat_EC_FIX, the LTP-specific 5-vector matrix-weighted
// entropy-constrained nearest-neighbor search).
package ltp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// LTPOrder mirrors LTP_ORDER (= 5) from SKP_Silk_define.h.
const LTPOrder = 5

// VQWMatECFix — SKP_Silk_VQ_WMat_EC_FIX.
//
// Entropy-constrained matrix-weighted VQ over a hard-coded 5-element vector.
// Searches `cbQ14` (L × 5 int16, row-major) for the codebook vector
// minimising  μ·cl + (in - cb)' W (in - cb), where W (5×5 row-major Q18)
// is symmetric. Returns the best index and the associated rate-distortion
// cost (Q14).
//
//	ind         = best codebook index (output)
//	rateDistQ14 = best mu·cl + weighted distortion (output, Q14)
//	inQ14       = input vector (5 int16, Q14)
//	WQ18        = 5×5 weighting matrix, row-major, Q18, symmetric
//	cbQ14       = codebook (L × LTP_ORDER int16, Q14)
//	clQ6        = code-length per row (L int16, Q6)
//	muQ8        = rate/distortion tradeoff (Q8)
//	L           = number of rows in cbQ14
//
// The C source has both a packed-int32 little-endian path and an unpacked
// big-endian path. The unpacked variant matches Go semantics directly and
// is bit-exact with the LE packed path on integer-arithmetic terms.
//
// Translated from vendor/silk/src/SKP_Silk_VQ_nearest_neighbor_FIX.c.
func VQWMatECFix(
	ind *int32,
	rateDistQ14 *int32,
	inQ14 []int16,
	WQ18 []int32,
	cbQ14 []int16,
	clQ6 []int16,
	muQ8 int32,
	L int32,
) {
	*rateDistQ14 = 0x7FFFFFFF
	for k := int32(0); k < L; k++ {
		row := cbQ14[k*LTPOrder:]
		var d [LTPOrder]int32
		d[0] = int32(inQ14[0]) - int32(row[0])
		d[1] = int32(inQ14[1]) - int32(row[1])
		d[2] = int32(inQ14[2]) - int32(row[2])
		d[3] = int32(inQ14[3]) - int32(row[3])
		d[4] = int32(inQ14[4]) - int32(row[4])

		// Weighted rate (Q14 = Q8 * Q6).
		sum1 := fix.SmulBB(muQ8, int32(clQ6[k]))

		// Row 0 of W: W[0..4].
		s2 := fix.SmulWB(WQ18[1], d[1])
		s2 = fix.SmlaWB(s2, WQ18[2], d[2])
		s2 = fix.SmlaWB(s2, WQ18[3], d[3])
		s2 = fix.SmlaWB(s2, WQ18[4], d[4])
		s2 = fix.LShift32(s2, 1)
		s2 = fix.SmlaWB(s2, WQ18[0], d[0])
		sum1 = fix.SmlaWB(sum1, s2, d[0])

		// Row 1: W[6..9] (skip [5] = W[1,0] by symmetry).
		s2 = fix.SmulWB(WQ18[7], d[2])
		s2 = fix.SmlaWB(s2, WQ18[8], d[3])
		s2 = fix.SmlaWB(s2, WQ18[9], d[4])
		s2 = fix.LShift32(s2, 1)
		s2 = fix.SmlaWB(s2, WQ18[6], d[1])
		sum1 = fix.SmlaWB(sum1, s2, d[1])

		// Row 2: W[12..14].
		s2 = fix.SmulWB(WQ18[13], d[3])
		s2 = fix.SmlaWB(s2, WQ18[14], d[4])
		s2 = fix.LShift32(s2, 1)
		s2 = fix.SmlaWB(s2, WQ18[12], d[2])
		sum1 = fix.SmlaWB(sum1, s2, d[2])

		// Row 3: W[18..19].
		s2 = fix.SmulWB(WQ18[19], d[4])
		s2 = fix.LShift32(s2, 1)
		s2 = fix.SmlaWB(s2, WQ18[18], d[3])
		sum1 = fix.SmlaWB(sum1, s2, d[3])

		// Row 4: W[24].
		s2 = fix.SmulWB(WQ18[24], d[4])
		sum1 = fix.SmlaWB(sum1, s2, d[4])

		if sum1 < *rateDistQ14 {
			*rateDistQ14 = sum1
			*ind = k
		}
	}
}
