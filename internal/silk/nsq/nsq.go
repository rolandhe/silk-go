// Package nsq implements SILK's Noise Shape Quantizer — both the
// single-state path (NSQ) and the delayed-decision variant
// (NSQ_del_dec). The encoder picks one based on the complexity setting:
// NSQ at low complexity (1 state), NSQ_del_dec at medium and high.
//
// Translated from vendor/silk/src/SKP_Silk_NSQ.c (and NSQ_del_dec.c,
// added in a follow-up).
package nsq

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// State mirrors SKP_Silk_nsq_state. The encoder package owns the
// authoritative definition (enc.NSQState) — this is a structurally
// identical copy here so the nsq package can stay free of an enc-package
// import. The two layouts must stay in lockstep.
//
// Field mapping to SKP_Silk_nsq_state:
//
//	Xq            ↔ xq[2*MAX_FRAME_LENGTH]
//	SLTPShpQ10    ↔ sLTP_shp_Q10[2*MAX_FRAME_LENGTH]
//	SLPCQ14       ↔ sLPC_Q14[MAX_FRAME_LENGTH/NB_SUBFR + NSQ_LPC_BUF_LENGTH]
//	SAR2Q14       ↔ sAR2_Q14[MAX_SHAPE_LPC_ORDER]
//	SLFARShpQ12   ↔ sLF_AR_shp_Q12
//	LagPrev       ↔ lagPrev
//	SLTPBufIdx    ↔ sLTP_buf_idx
//	SLTPShpBufIdx ↔ sLTP_shp_buf_idx
//	RandSeed      ↔ rand_seed
//	PrevInvGainQ16↔ prev_inv_gain_Q16
//	RewhiteFlag   ↔ rewhite_flag
type State struct {
	Xq             [2 * silk.MaxFrameLength]int16
	SLTPShpQ10     [2 * silk.MaxFrameLength]int32
	SLPCQ14        [silk.MaxFrameLength/silk.NBSubFr + silk.NSQLPCBufLength]int32
	SAR2Q14        [silk.MaxShapeLPCOrder]int32
	SLFARShpQ12    int32
	LagPrev        int32
	SLTPBufIdx     int32
	SLTPShpBufIdx  int32
	RandSeed       int32
	PrevInvGainQ16 int32
	RewhiteFlag    int32
}

// FrameParams aggregates the per-frame inputs the C source threads
// through SKP_Silk_NSQ. Bundling them keeps the call site readable and
// avoids a 14-argument Go function.
type FrameParams struct {
	FrameLength       int32
	SubfrLength       int32
	PredictLPCOrder   int32
	ShapingLPCOrder   int32
	Sigtype           int32
	QuantOffsetType   int32
	LSFInterpFactorQ2 int32
	LambdaQ10         int32
	LTPScaleQ14       int32
	Seed              int32
	PitchL            [silk.NBSubFr]int32
	GainsQ16          [silk.NBSubFr]int32
	HarmShapeGainQ14  [silk.NBSubFr]int32
	TiltQ14           [silk.NBSubFr]int32
	LFShpQ14          [silk.NBSubFr]int32
}

// NSQ — SKP_Silk_NSQ.
//
// Per-frame noise-shape quantizer with per-subframe re-whitening of the
// LTP state. Drives `q[]` (the quantized excitation pulses) and updates
// `NSQ.Xq` (the synthesized output, kept around for the next frame's
// re-whitening).
//
// `predCoefQ12` is laid out as the encoder's [2][MAX_LPC_ORDER]: row 0 =
// first-half-frame LPC, row 1 = second half. Same for `ltpCoefQ14`
// (NB_SUBFR rows of LTP_ORDER) and `AR2Q13`.
//
// Translated from vendor/silk/src/SKP_Silk_NSQ.c.
func NSQ(
	NSQ *State,
	x []int16,
	q []int8,
	p *FrameParams,
	predCoefQ12 *[2][silk.MaxLPCOrder]int16,
	ltpCoefQ14 []int16,
	AR2Q13 []int16,
) {
	NSQ.RandSeed = p.Seed
	lag := NSQ.LagPrev

	offsetQ10 := int32(tables.Quantization_Offsets_Q10[p.Sigtype][p.QuantOffsetType])

	lsfInterpFlag := int32(1)
	if p.LSFInterpFactorQ2 == (1 << 2) {
		lsfInterpFlag = 0
	}

	NSQ.SLTPShpBufIdx = p.FrameLength
	NSQ.SLTPBufIdx = p.FrameLength
	pxqOff := p.FrameLength

	var sLTPQ16 [2 * silk.MaxFrameLength]int32
	var sLTP [2 * silk.MaxFrameLength]int16
	var filtState [silk.MaxLPCOrder]int32
	var xScQ10 [silk.MaxFrameLength / silk.NBSubFr]int32

	xOff := int32(0)
	qOff := int32(0)
	for k := int32(0); k < silk.NBSubFr; k++ {
		// Pick LPC half (k>>1 = 0/1) — but if interpolation is off, force second half.
		AHalfRow := (k >> 1) | (1 - lsfInterpFlag)
		AQ12 := predCoefQ12[AHalfRow][:p.PredictLPCOrder]
		BQ14 := ltpCoefQ14[k*silk.LTPOrder : (k+1)*silk.LTPOrder]
		ARShpQ13 := AR2Q13[k*silk.MaxShapeLPCOrder : (k+1)*silk.MaxShapeLPCOrder]

		harmShapeFIRPackedQ14 := fix.RShift32(p.HarmShapeGainQ14[k], 2) |
			fix.LShift32(fix.RShift32(p.HarmShapeGainQ14[k], 1), 16)

		NSQ.RewhiteFlag = 0
		if p.Sigtype == silk.SigTypeVoiced {
			lag = p.PitchL[k]

			// Re-whiten at the boundaries determined by the NLSF
			// interpolation flag (every subframe when on, else just k=0).
			if (k & (3 - fix.LShift32(lsfInterpFlag, 1))) == 0 {
				startIdx := p.FrameLength - lag - p.PredictLPCOrder - silk.LTPOrder/2

				for i := range filtState[:p.PredictLPCOrder] {
					filtState[i] = 0
				}
				dsp.MAPrediction(
					NSQ.Xq[startIdx+k*(p.FrameLength>>2):],
					AQ12,
					filtState[:p.PredictLPCOrder],
					sLTP[startIdx:],
					p.FrameLength-startIdx,
					p.PredictLPCOrder,
				)

				NSQ.RewhiteFlag = 1
				NSQ.SLTPBufIdx = p.FrameLength
			}
		}

		nsqScaleStates(NSQ, x[xOff:], xScQ10[:], p.SubfrLength, sLTP[:], sLTPQ16[:],
			k, p.LTPScaleQ14, p.GainsQ16[:], p.PitchL[:])

		noiseShapeQuantizer(NSQ, p.Sigtype, xScQ10[:], q[qOff:], NSQ.Xq[pxqOff:],
			sLTPQ16[:], AQ12, BQ14, ARShpQ13, lag,
			harmShapeFIRPackedQ14, p.TiltQ14[k], p.LFShpQ14[k], p.GainsQ16[k],
			p.LambdaQ10, offsetQ10, p.SubfrLength, p.ShapingLPCOrder, p.PredictLPCOrder)

		xOff += p.SubfrLength
		qOff += p.SubfrLength
		pxqOff += p.SubfrLength
	}

	NSQ.LagPrev = p.PitchL[silk.NBSubFr-1]

	// Slide the quantized output and shape buffers back by one frame so
	// the next call sees them as preceding-history.
	copy(NSQ.Xq[:p.FrameLength], NSQ.Xq[p.FrameLength:p.FrameLength*2])
	copy(NSQ.SLTPShpQ10[:p.FrameLength], NSQ.SLTPShpQ10[p.FrameLength:p.FrameLength*2])
}

// nsqScaleStates — SKP_Silk_nsq_scale_states.
//
// Bring all NSQ state buffers into the same Q domain as the upcoming
// scaled input by applying the new inv_gain_Q16. After re-whitening, the
// LTP state arrives in unscaled Q0 and is multiplied in here.
func nsqScaleStates(NSQ *State, x []int16, xScQ10 []int32, subfrLen int32,
	sLTP []int16, sLTPQ16 []int32, subfr, LTPScaleQ14 int32,
	gainsQ16 []int32, pitchL []int32) {

	g := gainsQ16[subfr]
	if g < 1 {
		g = 1
	}
	invGainQ16 := fix.Inverse32VarQ(g, 32)
	if invGainQ16 > 0x7FFF {
		invGainQ16 = 0x7FFF
	}
	lag := pitchL[subfr]

	if NSQ.RewhiteFlag != 0 {
		invGainQ32 := fix.LShift32(invGainQ16, 16)
		if subfr == 0 {
			invGainQ32 = fix.LShift32(fix.SmulWB(invGainQ32, LTPScaleQ14), 2)
		}
		for i := NSQ.SLTPBufIdx - lag - silk.LTPOrder/2; i < NSQ.SLTPBufIdx; i++ {
			sLTPQ16[i] = fix.SmulWB(invGainQ32, int32(sLTP[i]))
		}
	}

	if invGainQ16 != NSQ.PrevInvGainQ16 {
		gainAdjQ16 := fix.Div32VarQ(invGainQ16, NSQ.PrevInvGainQ16, 16)

		for i := NSQ.SLTPShpBufIdx - subfrLen*silk.NBSubFr; i < NSQ.SLTPShpBufIdx; i++ {
			NSQ.SLTPShpQ10[i] = fix.SmulWW(gainAdjQ16, NSQ.SLTPShpQ10[i])
		}

		if NSQ.RewhiteFlag == 0 {
			for i := NSQ.SLTPBufIdx - lag - silk.LTPOrder/2; i < NSQ.SLTPBufIdx; i++ {
				sLTPQ16[i] = fix.SmulWW(gainAdjQ16, sLTPQ16[i])
			}
		}

		NSQ.SLFARShpQ12 = fix.SmulWW(gainAdjQ16, NSQ.SLFARShpQ12)

		for i := int32(0); i < silk.NSQLPCBufLength; i++ {
			NSQ.SLPCQ14[i] = fix.SmulWW(gainAdjQ16, NSQ.SLPCQ14[i])
		}
		for i := int32(0); i < silk.MaxShapeLPCOrder; i++ {
			NSQ.SAR2Q14[i] = fix.SmulWW(gainAdjQ16, NSQ.SAR2Q14[i])
		}
	}

	// Scale input.
	for i := int32(0); i < subfrLen; i++ {
		xScQ10[i] = fix.RShift32(fix.SmulBB(int32(x[i]), invGainQ16), 6)
	}
	NSQ.PrevInvGainQ16 = invGainQ16
}

// noiseShapeQuantizer — SKP_Silk_noise_shape_quantizer (the inner-loop
// quantizer). Writes per-sample pulses to `q`, the scaled-back signal to
// `xq`, and threads the LPC + LTP + shape state through `NSQ` and
// `sLTPQ16`.
func noiseShapeQuantizer(
	NSQ *State,
	sigtype int32,
	xScQ10 []int32,
	q []int8,
	xq []int16,
	sLTPQ16 []int32,
	aQ12 []int16,
	bQ14 []int16,
	ARShpQ13 []int16,
	lag int32,
	harmShapeFIRPackedQ14 int32,
	tiltQ14 int32,
	LFShpQ14 int32,
	gainQ16 int32,
	lambdaQ10 int32,
	offsetQ10 int32,
	length int32,
	shapingLPCOrder int32,
	predictLPCOrder int32,
) {
	shpLagOff := NSQ.SLTPShpBufIdx - lag + silk.HarmShapeFIRTaps/2
	predLagOff := NSQ.SLTPBufIdx - lag + silk.LTPOrder/2
	psLPCOff := int32(silk.NSQLPCBufLength - 1)

	thr1Q10 := -1536 - fix.RShift32(lambdaQ10, 1)
	thr2Q10 := -512 - fix.RShift32(lambdaQ10, 1)
	thr2Q10 = fix.AddRShift32(thr2Q10, fix.SmulBB(offsetQ10, lambdaQ10), 10)
	thr3Q10 := 512 + fix.RShift32(lambdaQ10, 1)

	for i := int32(0); i < length; i++ {
		NSQ.RandSeed = fix.Rand(NSQ.RandSeed)
		dither := fix.RShift32(NSQ.RandSeed, 31)

		// Short-term prediction (portable form: one SmlaWB per coef).
		LPCPredQ10 := fix.SmulWB(NSQ.SLPCQ14[psLPCOff], int32(aQ12[0]))
		for j := int32(1); j < predictLPCOrder; j++ {
			LPCPredQ10 = fix.SmlaWB(LPCPredQ10, NSQ.SLPCQ14[psLPCOff-j], int32(aQ12[j]))
		}

		// Long-term prediction.
		var LTPPredQ14 int32
		if sigtype == silk.SigTypeVoiced {
			LTPPredQ14 = fix.SmulWB(sLTPQ16[predLagOff+0], int32(bQ14[0]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-1], int32(bQ14[1]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-2], int32(bQ14[2]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-3], int32(bQ14[3]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-4], int32(bQ14[4]))
			predLagOff++
		}

		// Noise shape AR feedback (with the sAR2_Q14 line shift).
		tmp2 := NSQ.SLPCQ14[psLPCOff]
		tmp1 := NSQ.SAR2Q14[0]
		NSQ.SAR2Q14[0] = tmp2
		nARQ10 := fix.SmulWB(tmp2, int32(ARShpQ13[0]))
		for j := int32(2); j < shapingLPCOrder; j += 2 {
			tmp2 = NSQ.SAR2Q14[j-1]
			NSQ.SAR2Q14[j-1] = tmp1
			nARQ10 = fix.SmlaWB(nARQ10, tmp1, int32(ARShpQ13[j-1]))
			tmp1 = NSQ.SAR2Q14[j+0]
			NSQ.SAR2Q14[j+0] = tmp2
			nARQ10 = fix.SmlaWB(nARQ10, tmp2, int32(ARShpQ13[j]))
		}
		NSQ.SAR2Q14[shapingLPCOrder-1] = tmp1
		nARQ10 = fix.SmlaWB(nARQ10, tmp1, int32(ARShpQ13[shapingLPCOrder-1]))

		nARQ10 = fix.RShift32(nARQ10, 1) // Q11 → Q10
		nARQ10 = fix.SmlaWB(nARQ10, NSQ.SLFARShpQ12, tiltQ14)

		// LF shaping: low half of LFShp_Q14 = MA coef, high half = AR coef.
		nLFQ10 := fix.LShift32(fix.SmulWB(NSQ.SLTPShpQ10[NSQ.SLTPShpBufIdx-1], LFShpQ14), 2)
		nLFQ10 = fix.SmlaWT(nLFQ10, NSQ.SLFARShpQ12, LFShpQ14)

		// Long-term shaping with packed harm-shape coefs.
		var nLTPQ14 int32
		if lag > 0 {
			nLTPQ14 = fix.SmulWB(NSQ.SLTPShpQ10[shpLagOff+0]+NSQ.SLTPShpQ10[shpLagOff-2],
				harmShapeFIRPackedQ14)
			nLTPQ14 = fix.SmlaWT(nLTPQ14, NSQ.SLTPShpQ10[shpLagOff-1], harmShapeFIRPackedQ14)
			nLTPQ14 = fix.LShift32(nLTPQ14, 6)
			shpLagOff++
		}

		// Combined prediction error r = x - LTP - LPC + n_AR + n_LF + n_LTP.
		tmpa := LTPPredQ14 - nLTPQ14
		tmpa = fix.RShift32(tmpa, 4)
		tmpa = tmpa + LPCPredQ10
		tmpa = tmpa - nARQ10
		tmpa = tmpa - nLFQ10
		rQ10 := xScQ10[i] - tmpa

		// Sign-flip via dither, subtract bias, clamp to ±64×1024.
		rQ10 = (rQ10 ^ dither) - dither
		rQ10 -= offsetQ10
		rQ10 = fix.Limit(rQ10, -(64 << 10), 64<<10)

		var qQ0, qQ10 int32
		switch {
		case rQ10 < thr2Q10:
			if rQ10 < thr1Q10 {
				qQ0 = fix.RShiftRound(fix.AddRShift32(rQ10, lambdaQ10, 1), 10)
				qQ10 = fix.LShift32(qQ0, 10)
			} else {
				qQ0 = -1
				qQ10 = -1024
			}
		case rQ10 > thr3Q10:
			qQ0 = fix.RShiftRound(fix.SubRShift32(rQ10, lambdaQ10, 1), 10)
			qQ10 = fix.LShift32(qQ0, 10)
		}
		q[i] = int8(qQ0)

		// Excitation + predictions.
		excQ10 := qQ10 + offsetQ10
		excQ10 = (excQ10 ^ dither) - dither

		LPCExcQ10 := excQ10 + fix.RShiftRound(LTPPredQ14, 4)
		xqQ10 := LPCExcQ10 + LPCPredQ10

		// Scale back to int16.
		xq[i] = int16(fix.Sat16(fix.RShiftRound(fix.SmulWW(xqQ10, gainQ16), 10)))

		// State updates.
		psLPCOff++
		NSQ.SLPCQ14[psLPCOff] = fix.LShift32(xqQ10, 4)
		sLFARShpQ10 := xqQ10 - nARQ10
		NSQ.SLFARShpQ12 = fix.LShift32(sLFARShpQ10, 2)

		NSQ.SLTPShpQ10[NSQ.SLTPShpBufIdx] = sLFARShpQ10 - nLFQ10
		sLTPQ16[NSQ.SLTPBufIdx] = fix.LShift32(LPCExcQ10, 6)
		NSQ.SLTPShpBufIdx++
		NSQ.SLTPBufIdx++

		NSQ.RandSeed += int32(q[i])
	}

	// Slide LPC delay line forward by `length` samples.
	copy(NSQ.SLPCQ14[:silk.NSQLPCBufLength], NSQ.SLPCQ14[length:length+silk.NSQLPCBufLength])
}
