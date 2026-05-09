package ltp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// NBSubFr mirrors NB_SUBFR (= 4) from SKP_Silk_define.h.
const NBSubFr = 4

// AnalysisFilter — SKP_Silk_LTP_analysis_filter_FIX.
//
// Compute the LTP residual: for each subframe, subtract the long-term
// prediction (a 5-tap FIR over x[xStart-pitchL+k]) from x and scale by
// the inverse quantization gain.
//
// The C source takes a `const SKP_int16 *x` pointing into a buffer that
// has at least max(pitchL) preceding samples; the function reaches back
// via x[-pitchL]. Go slices can't be indexed negatively, so we ask the
// caller to pass the full buffer in `xBuf` and an offset `xStart` to the
// "current" position (= what the C source treats as x[0]).
//
//	ltpRes        — output residual, length NBSubFr*(preLength + subfrLength)
//	xBuf          — input buffer; samples xBuf[xStart-max(pitchL):] are the
//	                preceding-history; xBuf[xStart:] is the current data.
//	xStart        — offset into xBuf of subframe 0's first "current" sample.
//	ltpCoefQ14    — Q14 LTP coefficients [LTPOrder * NBSubFr]
//	pitchL        — per-subframe pitch lags [NBSubFr]
//	invGainsQ16   — per-subframe inverse quantization gains (Q16)
//	subfrLength   — samples per subframe
//	preLength     — preceding samples per subframe
//
// Translated from vendor/silk/src/SKP_Silk_LTP_analysis_filter_FIX.c.
func AnalysisFilter(
	ltpRes []int16,
	xBuf []int16,
	xStart int32,
	ltpCoefQ14 []int16,
	pitchL []int32,
	invGainsQ16 []int32,
	subfrLength int32,
	preLength int32,
) {
	xOff := xStart
	resOff := int32(0)
	for k := int32(0); k < NBSubFr; k++ {
		var b [LTPOrder]int16
		for i := int32(0); i < LTPOrder; i++ {
			b[i] = ltpCoefQ14[k*LTPOrder+i]
		}
		lagOff := xOff - pitchL[k]

		for i := int32(0); i < subfrLength+preLength; i++ {
			// LTP estimate: 5-tap FIR centered at xBuf[lagOff+LTPOrder/2].
			ltpEst := fix.SmulBB(int32(xBuf[lagOff+LTPOrder/2]), int32(b[0]))
			for j := int32(1); j < LTPOrder; j++ {
				ltpEst = fix.SmlaBBOvflw(ltpEst,
					int32(xBuf[lagOff+LTPOrder/2-j]), int32(b[j]))
			}
			ltpEst = fix.RShiftRound(ltpEst, 14)

			diff := fix.Sat16(int32(xBuf[xOff+i]) - ltpEst)
			ltpRes[resOff+i] = int16(fix.SmulWB(invGainsQ16[k], diff))

			lagOff++
		}

		resOff += subfrLength + preLength
		xOff += subfrLength
	}
}
