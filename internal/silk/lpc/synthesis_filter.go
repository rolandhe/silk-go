package lpc

import "github.com/rolandhe/silk-go/internal/silk/fix"

// SynthesisFilter — SKP_Silk_LPC_synthesis_filter.
// Even-order AR synthesis filter with state vector S (Q14), AR coefficients
// AQ12 (Q12), and Q26 gain on the excitation. The C version packs two
// int16 coefficients into an int32 to use SMLAWB/SMLAWT in tandem on ARM;
// the Go port uses the equivalent unpacked form (the big-endian branch in
// the C source).
//
// Translated from vendor/silk/src/SKP_Silk_LPC_synthesis_filter.c.
func SynthesisFilter(in []int16, AQ12 []int16, gainQ26 int32, S []int32, out []int16, length int32, order int32) {
	orderHalf := order >> 1
	// order must be even
	for k := int32(0); k < length; k++ {
		SA := S[order-1]
		var out32Q10 int32
		for j := int32(0); j < orderHalf-1; j++ {
			idx := 2*j + 1
			SB := S[order-1-idx]
			S[order-1-idx] = SA
			out32Q10 = fix.SmlaWB(out32Q10, SA, int32(AQ12[2*j]))
			out32Q10 = fix.SmlaWB(out32Q10, SB, int32(AQ12[2*j+1]))
			SA = S[order-2-idx]
			S[order-2-idx] = SB
		}
		// Epilog
		SB := S[0]
		S[0] = SA
		out32Q10 = fix.SmlaWB(out32Q10, SA, int32(AQ12[order-2]))
		out32Q10 = fix.SmlaWB(out32Q10, SB, int32(AQ12[order-1]))

		out32Q10 = fix.AddSat32(out32Q10, fix.SmulWB(gainQ26, int32(in[k])))
		out32 := fix.RShiftRound(out32Q10, 10)
		out[k] = int16(fix.Sat16(out32))
		S[order-1] = fix.LShiftSat32(out32Q10, 4)
	}
}
