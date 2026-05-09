package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Burg-modified constants — mirror SKP_Silk_burg_modified.c.
const (
	burgQA            = 25
	burgNBitsHeadRoom = 2
	burgMinRShifts    = -16
	burgMaxRShifts    = 32 - burgQA // = 7
)

// BurgModified — SKP_Silk_burg_modified.
//
// Compute reflection coefficients (output as Q16 prediction coefficients)
// from `x`. The input is `nbSubfr` concatenated sub-vectors, each of length
// `subfrLength` (which already includes the D preceding samples). Returns
// the residual energy with its Q value via res_nrg and resNrgQ.
//
// Translated from vendor/silk/src/SKP_Silk_burg_modified.c.
func BurgModified(
	resNrg *int32,
	resNrgQ *int32,
	AQ16 []int32,
	x []int16,
	subfrLength int32,
	nbSubfr int32,
	whiteNoiseFracQ32 int32,
	D int32,
) {
	var (
		CFirstRow [maxOrderLPC]int32
		CLastRow  [maxOrderLPC]int32
		AfQA      [maxOrderLPC]int32
		CAf       [maxOrderLPC + 1]int32
		CAb       [maxOrderLPC + 1]int32
	)

	var C0, rshifts int32
	C0Tmp, rshiftsTmp := SumSqrShift(x, nbSubfr*subfrLength)
	C0 = C0Tmp
	rshifts = rshiftsTmp

	if rshifts > burgMaxRShifts {
		C0 = fix.LShift32(C0, rshifts-burgMaxRShifts)
		rshifts = burgMaxRShifts
	} else {
		lz := fix.Clz32(C0) - 1
		rshiftsExtra := int32(burgNBitsHeadRoom) - lz
		if rshiftsExtra > 0 {
			if rshiftsExtra > burgMaxRShifts-rshifts {
				rshiftsExtra = burgMaxRShifts - rshifts
			}
			C0 = fix.RShift32(C0, rshiftsExtra)
		} else {
			if rshiftsExtra < burgMinRShifts-rshifts {
				rshiftsExtra = burgMinRShifts - rshifts
			}
			C0 = fix.LShift32(C0, -rshiftsExtra)
		}
		rshifts += rshiftsExtra
	}

	if rshifts > 0 {
		for s := int32(0); s < nbSubfr; s++ {
			xPtr := x[s*subfrLength:]
			for n := int32(1); n < D+1; n++ {
				CFirstRow[n-1] += int32(fix.RShift64(
					InnerProd16Aligned64(xPtr, xPtr[n:], subfrLength-n), rshifts))
			}
		}
	} else {
		for s := int32(0); s < nbSubfr; s++ {
			xPtr := x[s*subfrLength:]
			for n := int32(1); n < D+1; n++ {
				CFirstRow[n-1] += fix.LShift32(
					InnerProdAligned(xPtr, xPtr[n:], subfrLength-n), -rshifts)
			}
		}
	}
	CLastRow = CFirstRow

	CAb[0] = C0 + fix.Smmul(whiteNoiseFracQ32, C0) + 1
	CAf[0] = CAb[0]

	for n := int32(0); n < D; n++ {
		// Two precision branches based on rshifts.
		if rshifts > -2 {
			for s := int32(0); s < nbSubfr; s++ {
				xPtr := x[s*subfrLength:]
				x1 := -fix.LShift32(int32(xPtr[n]), 16-rshifts)
				x2 := -fix.LShift32(int32(xPtr[subfrLength-n-1]), 16-rshifts)
				tmp1 := fix.LShift32(int32(xPtr[n]), burgQA-16)
				tmp2 := fix.LShift32(int32(xPtr[subfrLength-n-1]), burgQA-16)
				for k := int32(0); k < n; k++ {
					CFirstRow[k] = fix.SmlaWB(CFirstRow[k], x1, int32(xPtr[n-k-1]))
					CLastRow[k] = fix.SmlaWB(CLastRow[k], x2, int32(xPtr[subfrLength-n+k]))
					AtmpQA := AfQA[k]
					tmp1 = fix.SmlaWB(tmp1, AtmpQA, int32(xPtr[n-k-1]))
					tmp2 = fix.SmlaWB(tmp2, AtmpQA, int32(xPtr[subfrLength-n+k]))
				}
				tmp1 = fix.LShift32(-tmp1, 32-burgQA-rshifts)
				tmp2 = fix.LShift32(-tmp2, 32-burgQA-rshifts)
				for k := int32(0); k <= n; k++ {
					CAf[k] = fix.SmlaWB(CAf[k], tmp1, int32(xPtr[n-k]))
					CAb[k] = fix.SmlaWB(CAb[k], tmp2, int32(xPtr[subfrLength-n+k-1]))
				}
			}
		} else {
			for s := int32(0); s < nbSubfr; s++ {
				xPtr := x[s*subfrLength:]
				x1 := -fix.LShift32(int32(xPtr[n]), -rshifts)
				x2 := -fix.LShift32(int32(xPtr[subfrLength-n-1]), -rshifts)
				tmp1 := fix.LShift32(int32(xPtr[n]), 17)
				tmp2 := fix.LShift32(int32(xPtr[subfrLength-n-1]), 17)
				for k := int32(0); k < n; k++ {
					CFirstRow[k] = fix.Mla(CFirstRow[k], x1, int32(xPtr[n-k-1]))
					CLastRow[k] = fix.Mla(CLastRow[k], x2, int32(xPtr[subfrLength-n+k]))
					Atmp1 := fix.RShiftRound(AfQA[k], burgQA-17)
					tmp1 = fix.Mla(tmp1, int32(xPtr[n-k-1]), Atmp1)
					tmp2 = fix.Mla(tmp2, int32(xPtr[subfrLength-n+k]), Atmp1)
				}
				tmp1 = -tmp1
				tmp2 = -tmp2
				for k := int32(0); k <= n; k++ {
					CAf[k] = fix.SmlaWW(CAf[k], tmp1,
						fix.LShift32(int32(xPtr[n-k]), -rshifts-1))
					CAb[k] = fix.SmlaWW(CAb[k], tmp2,
						fix.LShift32(int32(xPtr[subfrLength-n+k-1]), -rshifts-1))
				}
			}
		}

		// Numerator and denominator for the next reflection coefficient.
		tmp1 := CFirstRow[n]
		tmp2 := CLastRow[n]
		var num int32
		nrg := CAb[0] + CAf[0]
		for k := int32(0); k < n; k++ {
			AtmpQA := AfQA[k]
			lz := fix.Clz32(fix.Abs32(AtmpQA)) - 1
			if lz > 32-burgQA {
				lz = 32 - burgQA
			}
			Atmp1 := fix.LShift32(AtmpQA, lz)
			shift := 32 - burgQA - lz

			tmp1 = fix.AddLShift32(tmp1, fix.Smmul(CLastRow[n-k-1], Atmp1), shift)
			tmp2 = fix.AddLShift32(tmp2, fix.Smmul(CFirstRow[n-k-1], Atmp1), shift)
			num = fix.AddLShift32(num, fix.Smmul(CAb[n-k], Atmp1), shift)
			nrg = fix.AddLShift32(nrg, fix.Smmul(CAb[k+1]+CAf[k+1], Atmp1), shift)
		}
		CAf[n+1] = tmp1
		CAb[n+1] = tmp2
		num += tmp2
		num = fix.LShift32(-num, 1)

		// Reflection coefficient.
		var rcQ31 int32
		if fix.Abs32(num) < nrg {
			rcQ31 = fix.Div32VarQ(num, nrg, 31)
		} else {
			// Negative energy or |num/nrg| ≥ 1: zero the rest and exit.
			for k := n; k < D; k++ {
				AfQA[k] = 0
			}
			break
		}

		// Update AR coefficients (in-place, symmetric pair update).
		for k := int32(0); k < (n+1)>>1; k++ {
			t1 := AfQA[k]
			t2 := AfQA[n-k-1]
			AfQA[k] = fix.AddLShift32(t1, fix.Smmul(t2, rcQ31), 1)
			AfQA[n-k-1] = fix.AddLShift32(t2, fix.Smmul(t1, rcQ31), 1)
		}
		AfQA[n] = fix.RShift32(rcQ31, 31-burgQA)

		// Update C * Af and C * Ab.
		for k := int32(0); k <= n+1; k++ {
			t1 := CAf[k]
			t2 := CAb[n-k+1]
			CAf[k] = fix.AddLShift32(t1, fix.Smmul(t2, rcQ31), 1)
			CAb[n-k+1] = fix.AddLShift32(t2, fix.Smmul(t1, rcQ31), 1)
		}
	}

	// Residual energy + Q16 AR output.
	nrg := CAf[0]
	tmp1 := int32(1) << 16
	for k := int32(0); k < D; k++ {
		Atmp1 := fix.RShiftRound(AfQA[k], burgQA-16)
		nrg = fix.SmlaWW(nrg, CAf[k+1], Atmp1)
		tmp1 = fix.SmlaWW(tmp1, Atmp1, Atmp1)
		AQ16[k] = -Atmp1
	}
	*resNrg = fix.SmlaWW(nrg, fix.Smmul(whiteNoiseFracQ32, C0), -tmp1)
	*resNrgQ = -rshifts
}
