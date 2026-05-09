package ltp

import (
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// LTP find constants — mirror SKP_Silk_find_LTP_FIX.c.
const (
	ltpCorrsHeadRoom = 2

	// SKP_FIX_CONST(LTP_DAMPING/3, 16) = round(0.01/3 * 2^16) = 218.
	ltpDampingDiv3Q16 = int32(218)

	// SKP_FIX_CONST(LTP_SMOOTHING, 26) = round(0.1 * 2^26) = 6710886.
	ltpSmoothingQ26 = int32(6710886)
)

// fitLTP — SKP_Silk_fit_LTP. Convert Q16 LTP coefs to Q14 with rounding/saturation.
func fitLTP(coefsQ16 []int32, coefsQ14 []int16) {
	for i := int32(0); i < LTPOrder; i++ {
		coefsQ14[i] = int16(fix.Sat16(fix.RShiftRound(coefsQ16[i], 2)))
	}
}

// FindLTP — SKP_Silk_find_LTP_FIX.
//
// Estimate per-subframe LTP coefficients (b_Q14) and the weighting matrix
// (WLTP) used for quantization. Optionally returns the LTP coding gain.
//
// The C source takes raw `const SKP_int16 *r_first/*r_last` plus `mem_offset`;
// the function reads back as far as r_first[mem_offset - max(lag) - LTP_ORDER/2].
// Go slices can't index negatively, so the caller must pass the full buffer.
//
//	bQ14         — output [NBSubFr * LTPOrder]
//	WLTP         — output [NBSubFr * LTPOrder * LTPOrder]
//	ltpredCodGainQ7 — optional output, can be nil
//	rFirst       — full residual buffer for first 10ms (incl. memory)
//	rLast        — full residual buffer for last 10ms (incl. memory)
//	lag          — per-subframe LTP lags [NBSubFr]
//	wghtQ15      — per-subframe weights [NBSubFr]
//	subfrLength  — per-subframe samples
//	memOffset    — offset into rFirst/rLast where the "current" data begins
//	corrRshifts  — output [NBSubFr]: right shifts applied to each subframe's correlations
//
// Translated from vendor/silk/src/SKP_Silk_find_LTP_FIX.c.
func FindLTP(
	bQ14 []int16,
	WLTP []int32,
	ltpredCodGainQ7 *int32,
	rFirst []int16,
	rLast []int16,
	lag []int32,
	wghtQ15 []int32,
	subfrLength int32,
	memOffset int32,
	corrRshifts []int32,
) {
	var Rr [LTPOrder]int32
	var rr [NBSubFr]int32
	var nrg [NBSubFr]int32
	var dQ14 [NBSubFr]int32
	var w [NBSubFr]int32

	for k := int32(0); k < NBSubFr; k++ {
		// Pick which residual buffer this subframe lives in. The C source
		// advances `r_ptr += subfr_length` per iteration and resets to
		// `&r_last[mem_offset]` at k == NB_SUBFR/2 — so subframe k inside
		// each 10ms half starts `(k % (NB_SUBFR/2)) * subfr_length` past
		// `mem_offset` in the corresponding buffer.
		var rBuf []int16
		var rOff int32
		if k < NBSubFr/2 {
			rBuf = rFirst
			rOff = memOffset + k*subfrLength
		} else {
			rBuf = rLast
			rOff = memOffset + (k-NBSubFr/2)*subfrLength
		}
		rPtr := rBuf[rOff:]
		lagOff := rOff - (lag[k] + LTPOrder/2)
		lagPtr := rBuf[lagOff:]

		// Energy of r over this subframe (Q-rrShifts).
		rrK, rrShifts := dsp.SumSqrShift(rPtr, subfrLength)

		// Ensure head-room.
		if lz := fix.Clz32(rrK); lz < ltpCorrsHeadRoom {
			rrK = fix.RShiftRound(rrK, ltpCorrsHeadRoom-lz)
			rrShifts += ltpCorrsHeadRoom - lz
		}
		corrRshifts[k] = rrShifts
		rr[k] = rrK

		WLTPPtr := WLTP[k*LTPOrder*LTPOrder:]
		bQ14Ptr := bQ14[k*LTPOrder:]

		// Build the X'X correlation matrix.
		dsp.CorrMatrix(lagPtr, subfrLength, LTPOrder, ltpCorrsHeadRoom,
			WLTPPtr[:LTPOrder*LTPOrder], &corrRshifts[k])

		// X'·t correlation vector.
		dsp.CorrVector(lagPtr, rPtr, subfrLength, LTPOrder, Rr[:], corrRshifts[k])

		// Realign rr[k] if corrMatrix bumped its rshift further.
		if corrRshifts[k] > rrShifts {
			rr[k] = fix.RShift32(rr[k], corrRshifts[k]-rrShifts)
		}

		// Regularize the (correlation) Gram matrix to ensure positive
		// definiteness for solve_LDL.
		regu := int32(1)
		regu = fix.SmlaWB(regu, rr[k], ltpDampingDiv3Q16)
		regu = fix.SmlaWB(regu, WLTPPtr[0], ltpDampingDiv3Q16)
		regu = fix.SmlaWB(regu, WLTPPtr[(LTPOrder-1)*LTPOrder+(LTPOrder-1)], ltpDampingDiv3Q16)
		dsp.RegularizeCorrelations(WLTPPtr[:LTPOrder*LTPOrder], rr[k:k+1], regu, LTPOrder)

		// Solve b_Q16 = WLTP^-1 · Rr.
		var bQ16 [LTPOrder]int32
		dsp.SolveLDL(WLTPPtr[:LTPOrder*LTPOrder], LTPOrder, Rr[:], bQ16[:])

		// Q16 → Q14 with saturation.
		fitLTP(bQ16[:], bQ14Ptr[:LTPOrder])

		// Residual energy after LTP.
		nrg[k] = dsp.ResidualEnergy16Covar(bQ14Ptr[:LTPOrder],
			WLTPPtr[:LTPOrder*LTPOrder], Rr[:], rr[k], LTPOrder, 14)

		// Compute per-subframe weight w[k].
		extraShifts := corrRshifts[k]
		if ltpCorrsHeadRoom < extraShifts {
			extraShifts = ltpCorrsHeadRoom
		}
		denom := fix.LShiftSat32(fix.SmulWB(nrg[k], wghtQ15[k]), 1+extraShifts) +
			fix.RShift32(fix.SmulWB(subfrLength, 655), corrRshifts[k]-extraShifts)
		if denom < 1 {
			denom = 1
		}
		temp := fix.Div32(fix.LShift32(wghtQ15[k], 16), denom)
		temp = fix.RShift32(temp, 31+corrRshifts[k]-extraShifts-26)

		// Cap so the next scale doesn't overflow.
		var wMax int32
		for i := int32(0); i < LTPOrder*LTPOrder; i++ {
			if v := WLTPPtr[i]; v > wMax {
				wMax = v
			}
		}
		lshift := fix.Clz32(wMax) - 1 - 3
		if 26-18+lshift < 31 {
			cap := fix.LShift32(1, 26-18+lshift)
			if temp > cap {
				temp = cap
			}
		}

		dsp.ScaleVector32Q26Lshift18(WLTPPtr[:LTPOrder*LTPOrder], temp, LTPOrder*LTPOrder)
		w[k] = WLTPPtr[(LTPOrder>>1)*LTPOrder+(LTPOrder>>1)]
	}

	// Find max rshift across subframes.
	var maxRshifts int32
	for k := int32(0); k < NBSubFr; k++ {
		if corrRshifts[k] > maxRshifts {
			maxRshifts = corrRshifts[k]
		}
	}

	// LTP coding gain (optional).
	if ltpredCodGainQ7 != nil {
		var lpcResNrg, lpcLtpResNrg int32
		for k := int32(0); k < NBSubFr; k++ {
			shift := 1 + (maxRshifts - corrRshifts[k])
			lpcResNrg += fix.RShift32(fix.SmulWB(rr[k], wghtQ15[k])+1, shift)
			lpcLtpResNrg += fix.RShift32(fix.SmulWB(nrg[k], wghtQ15[k])+1, shift)
		}
		if lpcLtpResNrg < 1 {
			lpcLtpResNrg = 1
		}
		divQ16 := fix.Div32VarQ(lpcResNrg, lpcLtpResNrg, 16)
		*ltpredCodGainQ7 = fix.SmulBB(3, dsp.Lin2Log(divQ16)-(16<<7))
	}

	// Smoothing: d = sum(B, 1)
	for k := int32(0); k < NBSubFr; k++ {
		var s int32
		for i := int32(0); i < LTPOrder; i++ {
			s += int32(bQ14[k*LTPOrder+i])
		}
		dQ14[k] = s
	}

	// Find max abs d and bits used by w (in Q(18-maxRshifts)).
	var maxAbsD, maxWBits int32
	for k := int32(0); k < NBSubFr; k++ {
		if v := fix.Abs32(dQ14[k]); v > maxAbsD {
			maxAbsD = v
		}
		bits := 32 - fix.Clz32(w[k]) + corrRshifts[k] - maxRshifts
		if bits > maxWBits {
			maxWBits = bits
		}
	}

	extraShifts := maxWBits + 32 - fix.Clz32(maxAbsD) - 14
	extraShifts -= (32 - 1 - 2 + maxRshifts)
	if extraShifts < 0 {
		extraShifts = 0
	}
	maxRshiftsWxtra := maxRshifts + extraShifts

	// 1e-3f in Q(18 - maxRshifts_wxtra).
	temp := fix.RShift32(262, maxRshifts+extraShifts) + 1
	var wd int32
	for k := int32(0); k < NBSubFr; k++ {
		shifted := fix.RShift32(w[k], maxRshiftsWxtra-corrRshifts[k])
		temp += shifted
		wd += fix.LShift32(fix.SmulWW(shifted, dQ14[k]), 2)
	}
	mQ12 := fix.Div32VarQ(wd, temp, 12)

	for k := int32(0); k < NBSubFr; k++ {
		// w[k] from Q(18-corrRshifts[k]) to Q16.
		var t int32
		if 2-corrRshifts[k] > 0 {
			t = fix.RShift32(w[k], 2-corrRshifts[k])
		} else {
			t = fix.LShiftSat32(w[k], corrRshifts[k]-2)
		}

		gQ26 := fix.Mul(
			fix.Div32(ltpSmoothingQ26, fix.RShift32(ltpSmoothingQ26, 10)+t),
			fix.LShiftSat32(fix.SubSat32(mQ12, fix.RShift32(dQ14[k], 2)), 4),
		)

		var deltaB [LTPOrder]int16
		var sumDelta int32
		for i := int32(0); i < LTPOrder; i++ {
			v := bQ14[k*LTPOrder+i]
			if v < 1638 {
				v = 1638
			}
			deltaB[i] = v
			sumDelta += int32(v)
		}
		t = fix.Div32(gQ26, sumDelta)
		for i := int32(0); i < LTPOrder; i++ {
			updated := int32(bQ14[k*LTPOrder+i]) +
				fix.SmulWB(fix.LShiftSat32(t, 4), int32(deltaB[i]))
			if updated < -16000 {
				updated = -16000
			} else if updated > 28000 {
				updated = 28000
			}
			bQ14[k*LTPOrder+i] = int16(updated)
		}
	}
}
