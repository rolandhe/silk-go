package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Coefficients for the 2-band filter bank built from first-order all-pass
// sections. afb1Coef21 deliberately wraps around to a negative int16 — the
// C source casts `(SKP_int16)(20623 << 1)`, which is two's-complement
// 41246 - 65536 = -24290.
const (
	afb1Coef20 int16 = 5394 << 1
	afb1Coef21 int16 = -24290
)

// AnaFiltBank1 — SKP_Silk_ana_filt_bank_1.
//
// Splits the input signal into two decimated half-rate bands using a pair of
// first-order all-pass filters. Internal arithmetic is Q10. Writes the low
// band into outL and the high band into outH (both length N/2). The state
// vector S has two entries (one per all-pass section) and persists across
// calls.
//
// The C source carries a `scratch` parameter that is no longer used.
//
// Translated from vendor/silk/src/SKP_Silk_ana_filt_bank_1.c.
func AnaFiltBank1(in []int16, S []int32, outL, outH []int16, N int32) {
	N2 := fix.RShift32(N, 1)
	for k := int32(0); k < N2; k++ {
		// Even sample → first all-pass section.
		in32 := fix.LShift32(int32(in[2*k]), 10)
		Y := in32 - S[0]
		X := fix.SmlaWB(Y, Y, int32(afb1Coef21))
		out1 := S[0] + X
		S[0] = in32 + X

		// Odd sample → second all-pass section.
		in32 = fix.LShift32(int32(in[2*k+1]), 10)
		Y = in32 - S[1]
		X = fix.SmulWB(Y, int32(afb1Coef20))
		out2 := S[1] + X
		S[1] = in32 + X

		// Sum/difference → low/high band, back to int16.
		outL[k] = int16(fix.Sat16(fix.RShiftRound(out2+out1, 11)))
		outH[k] = int16(fix.Sat16(fix.RShiftRound(out2-out1, 11)))
	}
}
