// Package lpc ports the LPC step-up, inverse-prediction-gain and synthesis
// filters from vendor/silk/src/SKP_Silk_k2a*.c, SKP_Silk_LPC_inv_pred_gain.c
// and SKP_Silk_LPC_synthesis_*.c.
package lpc

import "github.com/rolandhe/silk-go/internal/silk/fix"

// MaxOrderLPC mirrors SKP_Silk_MAX_ORDER_LPC from SKP_Silk_SigProc_FIX.h.
const MaxOrderLPC = 16

// K2a — SKP_Silk_k2a. Step-up function: convert reflection coefficients (Q15)
// into prediction coefficients (Q24) in place at AQ24.
//
// Translated from vendor/silk/src/SKP_Silk_k2a.c.
func K2a(AQ24 []int32, rcQ15 []int16, order int32) {
	var atmp [MaxOrderLPC]int32
	for k := int32(0); k < order; k++ {
		for n := int32(0); n < k; n++ {
			atmp[n] = AQ24[n]
		}
		rc := int32(rcQ15[k])
		for n := int32(0); n < k; n++ {
			AQ24[n] = fix.SmlaWB(AQ24[n], fix.LShift32(atmp[k-n-1], 1), rc)
		}
		AQ24[k] = -fix.LShift32(rc, 9)
	}
}

// K2aQ16 — SKP_Silk_k2a_Q16. Same as K2a but reflection coefficients are Q16.
//
// Translated from vendor/silk/src/SKP_Silk_k2a_Q16.c.
func K2aQ16(AQ24 []int32, rcQ16 []int32, order int32) {
	var atmp [MaxOrderLPC]int32
	for k := int32(0); k < order; k++ {
		for n := int32(0); n < k; n++ {
			atmp[n] = AQ24[n]
		}
		rc := rcQ16[k]
		for n := int32(0); n < k; n++ {
			AQ24[n] = fix.SmlaWW(AQ24[n], atmp[k-n-1], rc)
		}
		AQ24[k] = -fix.LShift32(rc, 8)
	}
}
