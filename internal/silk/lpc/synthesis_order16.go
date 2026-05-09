package lpc

import "github.com/rolandhe/silk-go/internal/silk/fix"

// SynthesisOrder16 — SKP_Silk_LPC_synthesis_order16.
// Specialized 16th-order AR synthesis filter. The C source unrolls the inner
// loop and uses SMLAWB/SMLAWT_ovflw on packed int32 coefficients; the Go
// port keeps the unrolling but uses fix.SmlaWBOvflw against unpacked Q12
// coefficients. The wrapping additions are essential — the C original
// explicitly uses *_ovflw variants here.
//
// Translated from vendor/silk/src/SKP_Silk_LPC_synthesis_order16.c.
func SynthesisOrder16(in []int16, AQ12 []int16, gainQ26 int32, S []int32, out []int16, length int32) {
	for k := int32(0); k < length; k++ {
		SA := S[15]
		SB := S[14]
		S[14] = SA
		out32Q10 := fix.SmulWB(SA, int32(AQ12[0]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[1]))
		SA = S[13]
		S[13] = SB

		SB = S[12]
		S[12] = SA
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SA, int32(AQ12[2]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[3]))
		SA = S[11]
		S[11] = SB

		SB = S[10]
		S[10] = SA
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SA, int32(AQ12[4]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[5]))
		SA = S[9]
		S[9] = SB

		SB = S[8]
		S[8] = SA
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SA, int32(AQ12[6]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[7]))
		SA = S[7]
		S[7] = SB

		SB = S[6]
		S[6] = SA
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SA, int32(AQ12[8]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[9]))
		SA = S[5]
		S[5] = SB

		SB = S[4]
		S[4] = SA
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SA, int32(AQ12[10]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[11]))
		SA = S[3]
		S[3] = SB

		SB = S[2]
		S[2] = SA
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SA, int32(AQ12[12]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[13]))
		SA = S[1]
		S[1] = SB

		SB = S[0]
		S[0] = SA
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SA, int32(AQ12[14]))
		out32Q10 = fix.SmlaWBOvflw(out32Q10, SB, int32(AQ12[15]))

		out32Q10 = fix.AddSat32(out32Q10, fix.SmulWB(gainQ26, int32(in[k])))
		out32 := fix.RShiftRound(out32Q10, 10)
		out[k] = int16(fix.Sat16(out32))
		S[15] = fix.LShiftSat32(out32Q10, 4)
	}
}
