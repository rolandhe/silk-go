package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// decodeShortTermPrediction — SKP_Silk_decode_short_term_prediction.
//
// Per-sample LPC prediction: vec_Q10[i] = pres_Q10[i] + sum_{j=0..order-1}
// (sLPC_Q14[MaxLPCOrder + i - j - 1] * A_Q12_tmp[j]) >> 16. Updates sLPC_Q14
// at offset MaxLPCOrder + i with vec_Q10[i] << 4 (overflow allowed).
//
// The C source has a packed-int32 LE path that reads two coefs per int32;
// the Go port uses the equivalent unpacked SmlaWB sequence which is
// bit-identical (modulo the explicit overflow-allowed shift on the state
// store, which we match).
func decodeShortTermPrediction(vecQ10, presQ10, sLPCQ14 []int32, AQ12Tmp []int16, lpcOrder, subfrLength int32) {
	for i := int32(0); i < subfrLength; i++ {
		var pred int32
		for j := int32(0); j < lpcOrder; j++ {
			pred = fix.SmlaWB(pred, sLPCQ14[silk.MaxLPCOrder+i-j-1], int32(AQ12Tmp[j]))
		}
		vecQ10[i] = presQ10[i] + pred
		sLPCQ14[silk.MaxLPCOrder+i] = fix.LShiftOvflw32(vecQ10[i], 4)
	}
}

// DecodeCore — SKP_Silk_decode_core.
//
// Inverse NSQ + LTP + LPC synthesis: turn the entropy-decoded pulse
// sequence q[] into the final speech signal xq[].
//
// Translated from vendor/silk/src/SKP_Silk_decode_core.c.
func DecodeCore(psDec *State, ctrl *Control, xq []int16, q []int32) {
	if psDec.PrevInvGainQ16 == 0 {
		// SILK asserts non-zero; in practice production decoders never enter
		// here. Set a safe default so we don't divide by zero downstream.
		psDec.PrevInvGainQ16 = 1
	}

	offsetQ10 := int32(tables.Quantization_Offsets_Q10[ctrl.Sigtype][ctrl.QuantOffsetType])

	nlsfInterp := int32(0)
	if ctrl.NLSFInterpCoefQ2 < 4 {
		nlsfInterp = 1
	}

	// Decode excitation: pulses → exc_Q10 with sign dither from RNG.
	randSeed := ctrl.Seed
	for i := int32(0); i < psDec.FrameLength; i++ {
		randSeed = fix.Rand(randSeed)
		dither := fix.RShift32(randSeed, 31)
		v := fix.LShift32(q[i], 10) + offsetQ10
		v = (v ^ dither) - dither
		psDec.ExcQ10[i] = v
		randSeed += q[i]
	}

	pexcOff := int32(0)
	presOff := int32(0)
	pxqOff := psDec.FrameLength
	sLTPBufIdx := psDec.FrameLength

	var sLTP [silk.MaxFrameLength]int16
	var filtState [silk.MaxLPCOrder]int32
	var aQ12Tmp [silk.MaxLPCOrder]int16
	var vecQ10 [silk.MaxFrameLength / silk.NBSubFr]int32

	var lag int32

	for k := int32(0); k < silk.NBSubFr; k++ {
		// Pick LPC half (0 for first half-frame, 1 for second).
		AQ12 := ctrl.PredCoefQ12[k>>1][:psDec.LPCOrder]
		copy(aQ12Tmp[:psDec.LPCOrder], AQ12)

		BQ14 := ctrl.LTPCoefQ14[k*silk.LTPOrder : (k+1)*silk.LTPOrder]
		gainQ16 := ctrl.GainsQ16[k]
		sigtype := ctrl.Sigtype

		gainSafe := gainQ16
		if gainSafe < 1 {
			gainSafe = 1
		}
		invGainQ16 := fix.Inverse32VarQ(gainSafe, 32)
		if invGainQ16 > 0x7FFF {
			invGainQ16 = 0x7FFF
		}

		gainAdjQ16 := int32(1) << 16
		if invGainQ16 != psDec.PrevInvGainQ16 {
			gainAdjQ16 = fix.Div32VarQ(invGainQ16, psDec.PrevInvGainQ16, 16)
		}

		// Voiced-PLC → unvoiced transition guard.
		if psDec.LossCnt != 0 && psDec.PrevSigtype == silk.SigTypeVoiced &&
			ctrl.Sigtype == silk.SigTypeUnvoiced && k < silk.NBSubFr>>1 {
			for i := range BQ14 {
				BQ14[i] = 0
			}
			BQ14[silk.LTPOrder/2] = 1 << 12 // 0.25 in Q14
			sigtype = silk.SigTypeVoiced
			ctrl.PitchL[k] = psDec.LagPrev
		}

		if sigtype == silk.SigTypeVoiced {
			lag = ctrl.PitchL[k]
			// Re-whitening at boundaries determined by NLSF interpolation flag.
			if (k & (3 - fix.LShift32(nlsfInterp, 1))) == 0 {
				startIdx := psDec.FrameLength - lag - psDec.LPCOrder - silk.LTPOrder/2
				for i := range filtState {
					filtState[i] = 0
				}
				MAPredictionInt32(
					psDec.OutBuf[startIdx+k*(psDec.FrameLength>>2):],
					AQ12,
					filtState[:psDec.LPCOrder],
					sLTP[startIdx:],
					psDec.FrameLength-startIdx,
					psDec.LPCOrder,
				)

				invGainQ32 := fix.LShift32(invGainQ16, 16)
				if k == 0 {
					invGainQ32 = fix.LShift32(fix.SmulWB(invGainQ32, ctrl.LTPScaleQ14), 2)
				}
				for i := int32(0); i < lag+silk.LTPOrder/2; i++ {
					psDec.SLTPQ16[sLTPBufIdx-i-1] = fix.SmulWB(invGainQ32, int32(sLTP[psDec.FrameLength-i-1]))
				}
			} else {
				if gainAdjQ16 != int32(1)<<16 {
					for i := int32(0); i < lag+silk.LTPOrder/2; i++ {
						psDec.SLTPQ16[sLTPBufIdx-i-1] = fix.SmulWW(gainAdjQ16, psDec.SLTPQ16[sLTPBufIdx-i-1])
					}
				}
			}
		}

		// Re-scale short-term LPC state by the gain adjustment.
		for i := int32(0); i < silk.MaxLPCOrder; i++ {
			psDec.SLPCQ14[i] = fix.SmulWW(gainAdjQ16, psDec.SLPCQ14[i])
		}

		psDec.PrevInvGainQ16 = invGainQ16

		// LTP prediction (voiced) or copy excitation (unvoiced).
		if sigtype == silk.SigTypeVoiced {
			predLagOff := sLTPBufIdx - lag + silk.LTPOrder/2
			for i := int32(0); i < psDec.SubfrLength; i++ {
				ltpPred := fix.SmulWB(psDec.SLTPQ16[predLagOff+0], int32(BQ14[0]))
				ltpPred = fix.SmlaWB(ltpPred, psDec.SLTPQ16[predLagOff-1], int32(BQ14[1]))
				ltpPred = fix.SmlaWB(ltpPred, psDec.SLTPQ16[predLagOff-2], int32(BQ14[2]))
				ltpPred = fix.SmlaWB(ltpPred, psDec.SLTPQ16[predLagOff-3], int32(BQ14[3]))
				ltpPred = fix.SmlaWB(ltpPred, psDec.SLTPQ16[predLagOff-4], int32(BQ14[4]))
				predLagOff++

				psDec.ResQ10[presOff+i] = psDec.ExcQ10[pexcOff+i] + fix.RShiftRound(ltpPred, 4)
				psDec.SLTPQ16[sLTPBufIdx] = fix.LShift32(psDec.ResQ10[presOff+i], 6)
				sLTPBufIdx++
			}
		} else {
			copy(psDec.ResQ10[presOff:presOff+psDec.SubfrLength],
				psDec.ExcQ10[pexcOff:pexcOff+psDec.SubfrLength])
		}

		// Short-term prediction → vec_Q10.
		decodeShortTermPrediction(
			vecQ10[:psDec.SubfrLength],
			psDec.ResQ10[presOff:presOff+psDec.SubfrLength],
			psDec.SLPCQ14[:],
			aQ12Tmp[:psDec.LPCOrder],
			psDec.LPCOrder,
			psDec.SubfrLength,
		)

		// Scale with Gain → output.
		for i := int32(0); i < psDec.SubfrLength; i++ {
			psDec.OutBuf[pxqOff+i] = int16(fix.Sat16(fix.RShiftRound(fix.SmulWW(vecQ10[i], gainQ16), 10)))
		}

		// Slide LPC delay line forward by SubfrLength (the freshly written
		// portion at MaxLPCOrder..MaxLPCOrder+SubfrLength-1 becomes the new
		// "history" tail for the next subframe).
		copy(psDec.SLPCQ14[:silk.MaxLPCOrder], psDec.SLPCQ14[psDec.SubfrLength:psDec.SubfrLength+silk.MaxLPCOrder])

		pexcOff += psDec.SubfrLength
		presOff += psDec.SubfrLength
		pxqOff += psDec.SubfrLength
	}

	// Copy final output from the back half of OutBuf into xq.
	copy(xq[:psDec.FrameLength], psDec.OutBuf[psDec.FrameLength:psDec.FrameLength+psDec.FrameLength])
}

// MAPredictionInt32 — variant of MAPrediction that takes int32 state and
// writes int16 output (matches the C call site in decode_core: state is
// SKP_int32 FiltState[]).
func MAPredictionInt32(in []int16, B []int16, S []int32, out []int16, length, order int32) {
	for k := int32(0); k < length; k++ {
		in16 := int32(in[k])
		out32 := fix.LShift32(in16, 12) - S[0]
		out32 = fix.RShiftRound(out32, 12)

		for d := int32(0); d < order-1; d++ {
			S[d] = fix.SmlaBBOvflw(S[d+1], in16, int32(B[d]))
		}
		S[order-1] = fix.SmulBB(in16, int32(B[order-1]))

		out[k] = int16(fix.Sat16(out32))
	}
}
