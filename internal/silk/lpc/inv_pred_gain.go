package lpc

import "github.com/rolandhe/silk-go/internal/silk/fix"

// QA and ALimit mirror the file-static defines in SKP_Silk_LPC_inv_pred_gain.c.
const (
	qa = 16
	// SKP_FIX_CONST(0.99975, QA) = round(0.99975 * 2^16) = 65520.
	aLimit int32 = 65520
)

// inversePredGainQA implements the static helper LPC_inverse_pred_gain_QA.
// AQA is a 2x[order] scratch matrix where AQA[order&1] holds the input.
// Returns 1 if any reflection coefficient lies outside the unit circle.
func inversePredGainQA(invGainQ30 *int32, AQA *[2][MaxOrderLPC]int32, order int32) int32 {
	anewIdx := order & 1
	*invGainQ30 = 1 << 30

	for k := order - 1; k > 0; k-- {
		v := AQA[anewIdx][k]
		if v > aLimit || v < -aLimit {
			return 1
		}
		rcQ31 := -fix.LShift32(v, 31-qa)
		rcMult1Q30 := (int32(0x7FFFFFFF) >> 1) - fix.Smmul(rcQ31, rcQ31)
		rcMult2Q16 := fix.Inverse32VarQ(rcMult1Q30, 46)

		*invGainQ30 = fix.LShift32(fix.Smmul(*invGainQ30, rcMult1Q30), 2)

		aoldIdx := anewIdx
		anewIdx = k & 1

		headrm := fix.Clz32(rcMult2Q16) - 1
		rcMult2Q16 = fix.LShift32(rcMult2Q16, headrm)
		for n := int32(0); n < k; n++ {
			tmpQA := AQA[aoldIdx][n] - fix.LShift32(fix.Smmul(AQA[aoldIdx][k-n-1], rcQ31), 1)
			AQA[anewIdx][n] = fix.LShift32(fix.Smmul(tmpQA, rcMult2Q16), 16-headrm)
		}
	}

	v := AQA[anewIdx][0]
	if v > aLimit || v < -aLimit {
		return 1
	}
	rcQ31 := -fix.LShift32(v, 31-qa)
	rcMult1Q30 := (int32(0x7FFFFFFF) >> 1) - fix.Smmul(rcQ31, rcQ31)
	*invGainQ30 = fix.LShift32(fix.Smmul(*invGainQ30, rcMult1Q30), 2)
	return 0
}

// LPCInversePredGain — SKP_Silk_LPC_inverse_pred_gain. Input AQ12 is a
// length-`order` int16 prediction coefficient vector (Q12 domain).
// Writes invGainQ30 (Q30 inverse prediction gain), and returns 1 if the
// LPC poles are not all inside the unit circle (filter unstable).
//
// Translated from vendor/silk/src/SKP_Silk_LPC_inv_pred_gain.c.
func LPCInversePredGain(invGainQ30 *int32, AQ12 []int16, order int32) int32 {
	var atmp [2][MaxOrderLPC]int32
	idx := order & 1
	for k := int32(0); k < order; k++ {
		atmp[idx][k] = fix.LShift32(int32(AQ12[k]), qa-12)
	}
	return inversePredGainQA(invGainQ30, &atmp, order)
}

// LPCInversePredGainQ24 — SKP_Silk_LPC_inverse_pred_gain_Q24. Same as
// LPCInversePredGain but takes Q24-domain coefficients.
//
// Translated from vendor/silk/src/SKP_Silk_LPC_inv_pred_gain.c.
func LPCInversePredGainQ24(invGainQ30 *int32, AQ24 []int32, order int32) int32 {
	var atmp [2][MaxOrderLPC]int32
	idx := order & 1
	for k := int32(0); k < order; k++ {
		atmp[idx][k] = fix.RShiftRound(AQ24[k], 24-qa)
	}
	return inversePredGainQA(invGainQ30, &atmp, order)
}
