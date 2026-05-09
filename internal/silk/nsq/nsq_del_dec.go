package nsq

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// delDecStruct mirrors NSQ_del_dec_struct from the C source. Each
// candidate path through the delayed-decision tree carries its own LPC
// + AR2 state, RD score, and ring-buffered history (length DECISION_DELAY).
type delDecStruct struct {
	RandState [silk.DecisionDelay]int32
	QQ10      [silk.DecisionDelay]int32
	XqQ10     [silk.DecisionDelay]int32
	PredQ16   [silk.DecisionDelay]int32
	ShapeQ10  [silk.DecisionDelay]int32
	GainQ16   [silk.DecisionDelay]int32
	SAR2Q14   [silk.MaxShapeLPCOrder]int32
	SLPCQ14   [silk.MaxFrameLength/silk.NBSubFr + silk.NSQLPCBufLength]int32
	LFARQ12   int32
	Seed      int32
	SeedInit  int32
	RDQ10     int32
}

// nsqSampleStruct mirrors NSQ_sample_struct — one of two per-sample
// candidates considered for each path during del_dec quantization.
type nsqSampleStruct struct {
	QQ10       int32
	RDQ10      int32
	XqQ14      int32
	LFARQ12    int32
	SLTPShpQ10 int32
	LPCExcQ16  int32
}

// copyDelDecState — SKP_Silk_copy_del_dec_state.
//
// Clone every field except sLPC_Q14 below `lpcStateIdx` (the lower part
// of the LPC ring is still shared with the original path's history).
func copyDelDecState(dst, src *delDecStruct, lpcStateIdx int32) {
	dst.RandState = src.RandState
	dst.QQ10 = src.QQ10
	dst.PredQ16 = src.PredQ16
	dst.ShapeQ10 = src.ShapeQ10
	dst.XqQ10 = src.XqQ10
	dst.SAR2Q14 = src.SAR2Q14
	for i := lpcStateIdx; i < lpcStateIdx+silk.NSQLPCBufLength; i++ {
		dst.SLPCQ14[i] = src.SLPCQ14[i]
	}
	dst.LFARQ12 = src.LFARQ12
	dst.Seed = src.Seed
	dst.SeedInit = src.SeedInit
	dst.RDQ10 = src.RDQ10
}

// NSQDelDec — SKP_Silk_NSQ_del_dec.
//
// Delayed-decision noise-shape quantizer. Maintains
// `nStatesDelayedDecision` parallel candidate paths through the
// quantization tree, picks the lowest-RD survivor every sample, and
// emits the winner's pulses delayed by `decisionDelay` so an earlier
// state-flip can still re-pick a different quantized value.
//
// Used at complexity ≥ 1 (medium/high). Complexity 0 falls through to
// the simpler NSQ() above.
//
// `predCoefQ12`, `ltpCoefQ14`, `AR2Q13` follow the same packed layout
// as NSQ. `nStatesDelayedDecision` ∈ {1..MAX_DEL_DEC_STATES}; the
// encoder's complexity setup picks 2 (medium) or 4 (high).
//
// `seedOut` receives the SeedInit of the winning path on exit so the
// caller can mirror the C source's `psEncCtrlC->Seed = psDD->SeedInit`.
//
// Translated from vendor/silk/src/SKP_Silk_NSQ_del_dec.c.
func NSQDelDec(
	NSQ *State,
	x []int16,
	q []int8,
	p *FrameParams,
	predCoefQ12 *[2][silk.MaxLPCOrder]int16,
	ltpCoefQ14 []int16,
	AR2Q13 []int16,
	warpingQ16 int32,
	nStatesDelayedDecision int32,
	seedOut *int32,
) {
	subfrLength := p.FrameLength / silk.NBSubFr
	lag := NSQ.LagPrev

	// Initialize del_dec states.
	var psDelDec [silk.MaxDelDecStates]delDecStruct
	for k := int32(0); k < nStatesDelayedDecision; k++ {
		dd := &psDelDec[k]
		dd.Seed = (k + p.Seed) & 3
		dd.SeedInit = dd.Seed
		dd.LFARQ12 = NSQ.SLFARShpQ12
		dd.ShapeQ10[0] = NSQ.SLTPShpQ10[p.FrameLength-1]
		copy(dd.SLPCQ14[:silk.NSQLPCBufLength], NSQ.SLPCQ14[:silk.NSQLPCBufLength])
		dd.SAR2Q14 = NSQ.SAR2Q14
	}

	offsetQ10 := int32(tables.Quantization_Offsets_Q10[p.Sigtype][p.QuantOffsetType])
	smplBufIdx := int32(0)

	decisionDelay := fix.MinInt(silk.DecisionDelay, subfrLength)
	if p.Sigtype == silk.SigTypeVoiced {
		for k := int32(0); k < silk.NBSubFr; k++ {
			decisionDelay = fix.MinInt(decisionDelay, p.PitchL[k]-silk.LTPOrder/2-1)
		}
	} else if lag > 0 {
		decisionDelay = fix.MinInt(decisionDelay, lag-silk.LTPOrder/2-1)
	}

	lsfInterpFlag := int32(1)
	if p.LSFInterpFactorQ2 == (1 << 2) {
		lsfInterpFlag = 0
	}

	pxqOff := p.FrameLength
	NSQ.SLTPShpBufIdx = p.FrameLength
	NSQ.SLTPBufIdx = p.FrameLength

	var sLTPQ16 [2 * silk.MaxFrameLength]int32
	var sLTP [2 * silk.MaxFrameLength]int16
	var filtState [silk.MaxLPCOrder]int32
	var xScQ10 [silk.MaxFrameLength / silk.NBSubFr]int32

	subfrCounter := int32(0)
	xOff := int32(0)
	qOff := int32(0)

	for k := int32(0); k < silk.NBSubFr; k++ {
		AHalfRow := (k >> 1) | (1 - lsfInterpFlag)
		AQ12 := predCoefQ12[AHalfRow][:p.PredictLPCOrder]
		BQ14 := ltpCoefQ14[k*silk.LTPOrder : (k+1)*silk.LTPOrder]
		ARShpQ13 := AR2Q13[k*silk.MaxShapeLPCOrder : (k+1)*silk.MaxShapeLPCOrder]

		harmShapeFIRPackedQ14 := fix.RShift32(p.HarmShapeGainQ14[k], 2) |
			fix.LShift32(fix.RShift32(p.HarmShapeGainQ14[k], 1), 16)

		NSQ.RewhiteFlag = 0
		if p.Sigtype == silk.SigTypeVoiced {
			lag = p.PitchL[k]

			if (k & (3 - fix.LShift32(lsfInterpFlag, 1))) == 0 {
				if k == 2 {
					// Reset point: pick the current winner and freeze it
					// into the NSQ output / shape buffers, then re-arm
					// the search from there.
					RDmin := psDelDec[0].RDQ10
					winner := int32(0)
					for i := int32(1); i < nStatesDelayedDecision; i++ {
						if psDelDec[i].RDQ10 < RDmin {
							RDmin = psDelDec[i].RDQ10
							winner = i
						}
					}
					for i := int32(0); i < nStatesDelayedDecision; i++ {
						if i != winner {
							psDelDec[i].RDQ10 += 0x7FFFFFFF >> 4
						}
					}

					dd := &psDelDec[winner]
					lastIdx := smplBufIdx + decisionDelay
					for i := int32(0); i < decisionDelay; i++ {
						lastIdx = (lastIdx - 1) & silk.DecisionDelayMask
						q[qOff+i-decisionDelay] = int8(fix.RShift32(dd.QQ10[lastIdx], 10))
						NSQ.Xq[pxqOff+i-decisionDelay] = int16(fix.Sat16(
							fix.RShiftRound(fix.SmulWW(dd.XqQ10[lastIdx], dd.GainQ16[lastIdx]), 10)))
						NSQ.SLTPShpQ10[NSQ.SLTPShpBufIdx-decisionDelay+i] = dd.ShapeQ10[lastIdx]
					}
					subfrCounter = 0
				}

				// Re-whiten with new A coefs.
				startIdx := p.FrameLength - lag - p.PredictLPCOrder - silk.LTPOrder/2
				for i := range filtState[:p.PredictLPCOrder] {
					filtState[i] = 0
				}
				dsp.MAPrediction(
					NSQ.Xq[startIdx+k*subfrLength:],
					AQ12, filtState[:p.PredictLPCOrder],
					sLTP[startIdx:],
					p.FrameLength-startIdx, p.PredictLPCOrder,
				)

				NSQ.SLTPBufIdx = p.FrameLength
				NSQ.RewhiteFlag = 1
			}
		}

		nsqDelDecScaleStates(NSQ, psDelDec[:], x[xOff:], xScQ10[:],
			subfrLength, sLTP[:], sLTPQ16[:], k, nStatesDelayedDecision, smplBufIdx,
			p.LTPScaleQ14, p.GainsQ16[:], p.PitchL[:])

		smplBufIdx = noiseShapeQuantizerDelDec(NSQ, psDelDec[:], p.Sigtype, xScQ10[:],
			q, qOff, NSQ.Xq[:], pxqOff, sLTPQ16[:], AQ12, BQ14, ARShpQ13, lag,
			harmShapeFIRPackedQ14, p.TiltQ14[k], p.LFShpQ14[k], p.GainsQ16[k],
			p.LambdaQ10, offsetQ10, subfrLength, subfrCounter,
			p.ShapingLPCOrder, p.PredictLPCOrder, warpingQ16,
			nStatesDelayedDecision, smplBufIdx, decisionDelay)
		subfrCounter++

		xOff += subfrLength
		qOff += subfrLength
		pxqOff += subfrLength
	}

	// Final winner pick + flush remaining decisionDelay samples.
	RDmin := psDelDec[0].RDQ10
	winner := int32(0)
	for k := int32(1); k < nStatesDelayedDecision; k++ {
		if psDelDec[k].RDQ10 < RDmin {
			RDmin = psDelDec[k].RDQ10
			winner = k
		}
	}
	dd := &psDelDec[winner]
	if seedOut != nil {
		*seedOut = dd.SeedInit
	}
	lastIdx := smplBufIdx + decisionDelay
	// q and pxq pointers have advanced past the frame; fall back to the
	// negative-index slot in the underlying buffer.
	qBase := qOff - decisionDelay
	pxqBase := pxqOff - decisionDelay
	for i := int32(0); i < decisionDelay; i++ {
		lastIdx = (lastIdx - 1) & silk.DecisionDelayMask
		q[qBase+i] = int8(fix.RShift32(dd.QQ10[lastIdx], 10))
		NSQ.Xq[pxqBase+i] = int16(fix.Sat16(
			fix.RShiftRound(fix.SmulWW(dd.XqQ10[lastIdx], dd.GainQ16[lastIdx]), 10)))
		NSQ.SLTPShpQ10[NSQ.SLTPShpBufIdx-decisionDelay+i] = dd.ShapeQ10[lastIdx]
		sLTPQ16[NSQ.SLTPBufIdx-decisionDelay+i] = dd.PredQ16[lastIdx]
	}
	copy(NSQ.SLPCQ14[:silk.NSQLPCBufLength], dd.SLPCQ14[subfrLength:subfrLength+silk.NSQLPCBufLength])
	NSQ.SAR2Q14 = dd.SAR2Q14

	NSQ.SLFARShpQ12 = dd.LFARQ12
	NSQ.LagPrev = p.PitchL[silk.NBSubFr-1]

	copy(NSQ.Xq[:p.FrameLength], NSQ.Xq[p.FrameLength:p.FrameLength*2])
	copy(NSQ.SLTPShpQ10[:p.FrameLength], NSQ.SLTPShpQ10[p.FrameLength:p.FrameLength*2])
}

// noiseShapeQuantizerDelDec — SKP_Silk_noise_shape_quantizer_del_dec.
//
// One subframe of the inner del_dec loop. For each input sample:
//   - compute LTP + LTP-shape prediction (state-independent),
//   - then for each of `nStates` paths compute LPC pred + warped AR
//     shape feedback + LF tilt feedback, evaluate two candidate quantized
//     pulses and their RD,
//   - merge: find best path, demote rivals from the same RandState,
//     swap-in best second-candidate over worst first if it wins,
//   - emit the (decisionDelay-old) winner's sample to outputs.
//
// Returns the new smpl_buf_idx — the C source updates it via pointer; we
// pass it back as the return value and let the caller thread it through.
// `q` and `xq` are passed as full backing slices with explicit `qOff`
// and `xqOff` start markers so we can index back into samples emitted in
// the previous subframe — the C source uses pointer arithmetic for this
// (q[i - decisionDelay] when i < decisionDelay reaches behind the
// per-subframe pointer).
func noiseShapeQuantizerDelDec(
	NSQ *State,
	psDelDec []delDecStruct,
	sigtype int32,
	xQ10 []int32,
	q []int8,
	qOff int32,
	xq []int16,
	xqOff int32,
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
	subfr int32,
	shapingLPCOrder int32,
	predictLPCOrder int32,
	warpingQ16 int32,
	nStates int32,
	smplBufIdx int32,
	decisionDelay int32,
) int32 {
	var psSampleState [silk.MaxDelDecStates][2]nsqSampleStruct

	shpLagOff := NSQ.SLTPShpBufIdx - lag + silk.HarmShapeFIRTaps/2
	predLagOff := NSQ.SLTPBufIdx - lag + silk.LTPOrder/2

	for i := int32(0); i < length; i++ {
		// State-independent: LTP prediction and LTP-shape feedback.
		var LTPPredQ14 int32
		if sigtype == silk.SigTypeVoiced {
			LTPPredQ14 = fix.SmulWB(sLTPQ16[predLagOff+0], int32(bQ14[0]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-1], int32(bQ14[1]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-2], int32(bQ14[2]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-3], int32(bQ14[3]))
			LTPPredQ14 = fix.SmlaWB(LTPPredQ14, sLTPQ16[predLagOff-4], int32(bQ14[4]))
			predLagOff++
		}
		var nLTPQ14 int32
		if lag > 0 {
			nLTPQ14 = fix.SmulWB(NSQ.SLTPShpQ10[shpLagOff+0]+NSQ.SLTPShpQ10[shpLagOff-2], harmShapeFIRPackedQ14)
			nLTPQ14 = fix.SmlaWT(nLTPQ14, NSQ.SLTPShpQ10[shpLagOff-1], harmShapeFIRPackedQ14)
			nLTPQ14 = fix.LShift32(nLTPQ14, 6)
			shpLagOff++
		}

		for k := int32(0); k < nStates; k++ {
			dd := &psDelDec[k]
			ss := psSampleState[k][:]

			dd.Seed = fix.Rand(dd.Seed)
			dither := fix.RShift32(dd.Seed, 31)

			psLPCOff := int32(silk.NSQLPCBufLength - 1 + i)

			// Short-term LPC prediction.
			LPCPredQ10 := fix.SmulWB(dd.SLPCQ14[psLPCOff], int32(aQ12[0]))
			for j := int32(1); j < predictLPCOrder; j++ {
				LPCPredQ10 = fix.SmlaWB(LPCPredQ10, dd.SLPCQ14[psLPCOff-j], int32(aQ12[j]))
			}

			// Warped AR shape feedback (warping_Q16 = 0 ⇒ degrades to plain).
			tmp2 := fix.SmlaWB(dd.SLPCQ14[psLPCOff], dd.SAR2Q14[0], warpingQ16)
			tmp1 := fix.SmlaWB(dd.SAR2Q14[0], dd.SAR2Q14[1]-tmp2, warpingQ16)
			dd.SAR2Q14[0] = tmp2
			nARQ10 := fix.SmulWB(tmp2, int32(ARShpQ13[0]))
			for j := int32(2); j < shapingLPCOrder; j += 2 {
				tmp2 = fix.SmlaWB(dd.SAR2Q14[j-1], dd.SAR2Q14[j+0]-tmp1, warpingQ16)
				dd.SAR2Q14[j-1] = tmp1
				nARQ10 = fix.SmlaWB(nARQ10, tmp1, int32(ARShpQ13[j-1]))
				tmp1 = fix.SmlaWB(dd.SAR2Q14[j+0], dd.SAR2Q14[j+1]-tmp2, warpingQ16)
				dd.SAR2Q14[j+0] = tmp2
				nARQ10 = fix.SmlaWB(nARQ10, tmp2, int32(ARShpQ13[j]))
			}
			dd.SAR2Q14[shapingLPCOrder-1] = tmp1
			nARQ10 = fix.SmlaWB(nARQ10, tmp1, int32(ARShpQ13[shapingLPCOrder-1]))

			nARQ10 = fix.RShift32(nARQ10, 1)
			nARQ10 = fix.SmlaWB(nARQ10, dd.LFARQ12, tiltQ14)

			nLFQ10 := fix.LShift32(fix.SmulWB(dd.ShapeQ10[smplBufIdx], LFShpQ14), 2)
			nLFQ10 = fix.SmlaWT(nLFQ10, dd.LFARQ12, LFShpQ14)

			// Residual.
			tmp1a := LTPPredQ14 - nLTPQ14
			tmp1a = fix.RShift32(tmp1a, 4)
			tmp1a = tmp1a + LPCPredQ10
			tmp1a = tmp1a - nARQ10
			tmp1a = tmp1a - nLFQ10
			rQ10 := xQ10[i] - tmp1a

			rQ10 = (rQ10 ^ dither) - dither
			rQ10 -= offsetQ10
			rQ10 = fix.Limit(rQ10, -(64 << 10), 64<<10)

			// Two quantization candidates with RDs.
			var q1Q10, q2Q10, rd1Q10, rd2Q10 int32
			switch {
			case rQ10 < -1536:
				q1Q10 = fix.LShift32(fix.RShiftRound(rQ10, 10), 10)
				rQ10b := rQ10 - q1Q10
				rd1Q10 = fix.RShift32(fix.SmlaBB(-(q1Q10+offsetQ10)*lambdaQ10, rQ10b, rQ10b), 10)
				rd2Q10 = rd1Q10 + 1024
				rd2Q10 = rd2Q10 - fix.AddLShift32(lambdaQ10, rQ10b, 1)
				q2Q10 = q1Q10 + 1024
			case rQ10 > 512:
				q1Q10 = fix.LShift32(fix.RShiftRound(rQ10, 10), 10)
				rQ10b := rQ10 - q1Q10
				rd1Q10 = fix.RShift32(fix.SmlaBB((q1Q10+offsetQ10)*lambdaQ10, rQ10b, rQ10b), 10)
				rd2Q10 = rd1Q10 + 1024
				rd2Q10 = rd2Q10 - fix.SubLShift32(lambdaQ10, rQ10b, 1)
				q2Q10 = q1Q10 - 1024
			default:
				rrQ20 := fix.SmulBB(offsetQ10, lambdaQ10)
				rd2Q10 = fix.RShift32(fix.SmlaBB(rrQ20, rQ10, rQ10), 10)
				rd1Q10 = rd2Q10 + 1024
				rd1Q10 = rd1Q10 + fix.SubRShift32(fix.AddLShift32(lambdaQ10, rQ10, 1), rrQ20, 9)
				q1Q10 = -1024
				q2Q10 = 0
			}

			if rd1Q10 < rd2Q10 {
				ss[0].RDQ10 = dd.RDQ10 + rd1Q10
				ss[1].RDQ10 = dd.RDQ10 + rd2Q10
				ss[0].QQ10 = q1Q10
				ss[1].QQ10 = q2Q10
			} else {
				ss[0].RDQ10 = dd.RDQ10 + rd2Q10
				ss[1].RDQ10 = dd.RDQ10 + rd1Q10
				ss[0].QQ10 = q2Q10
				ss[1].QQ10 = q1Q10
			}

			// Build per-candidate state.
			for c := 0; c < 2; c++ {
				excQ10 := offsetQ10 + ss[c].QQ10
				excQ10 = (excQ10 ^ dither) - dither
				LPCExcQ10 := excQ10 + fix.RShiftRound(LTPPredQ14, 4)
				xqQ10 := LPCExcQ10 + LPCPredQ10
				sLFARShpQ10 := xqQ10 - nARQ10
				ss[c].SLTPShpQ10 = sLFARShpQ10 - nLFQ10
				ss[c].LFARQ12 = fix.LShift32(sLFARShpQ10, 2)
				ss[c].XqQ14 = fix.LShift32(xqQ10, 4)
				ss[c].LPCExcQ16 = fix.LShift32(LPCExcQ10, 6)
			}
		}

		smplBufIdx = (smplBufIdx - 1) & silk.DecisionDelayMask
		lastIdx := (smplBufIdx + decisionDelay) & silk.DecisionDelayMask

		// Find winner.
		RDmin := psSampleState[0][0].RDQ10
		winner := int32(0)
		for k := int32(1); k < nStates; k++ {
			if psSampleState[k][0].RDQ10 < RDmin {
				RDmin = psSampleState[k][0].RDQ10
				winner = k
			}
		}

		// Demote paths sharing the same RandState as winner's expired sample.
		winnerRand := psDelDec[winner].RandState[lastIdx]
		for k := int32(0); k < nStates; k++ {
			if psDelDec[k].RandState[lastIdx] != winnerRand {
				psSampleState[k][0].RDQ10 += 0x7FFFFFFF >> 4
				psSampleState[k][1].RDQ10 += 0x7FFFFFFF >> 4
			}
		}

		// Find worst-of-first-set, best-of-second-set.
		RDmaxQ10 := psSampleState[0][0].RDQ10
		RDminQ10 := psSampleState[0][1].RDQ10
		RDmaxInd := int32(0)
		RDminInd := int32(0)
		for k := int32(1); k < nStates; k++ {
			if psSampleState[k][0].RDQ10 > RDmaxQ10 {
				RDmaxQ10 = psSampleState[k][0].RDQ10
				RDmaxInd = k
			}
			if psSampleState[k][1].RDQ10 < RDminQ10 {
				RDminQ10 = psSampleState[k][1].RDQ10
				RDminInd = k
			}
		}

		if RDminQ10 < RDmaxQ10 {
			copyDelDecState(&psDelDec[RDmaxInd], &psDelDec[RDminInd], i)
			psSampleState[RDmaxInd][0] = psSampleState[RDminInd][1]
		}

		// Emit the (decisionDelay-old) winner's sample.
		dd := &psDelDec[winner]
		if subfr > 0 || i >= decisionDelay {
			q[qOff+i-decisionDelay] = int8(fix.RShift32(dd.QQ10[lastIdx], 10))
			xq[xqOff+i-decisionDelay] = int16(fix.Sat16(
				fix.RShiftRound(fix.SmulWW(dd.XqQ10[lastIdx], dd.GainQ16[lastIdx]), 10)))
			NSQ.SLTPShpQ10[NSQ.SLTPShpBufIdx-decisionDelay] = dd.ShapeQ10[lastIdx]
			sLTPQ16[NSQ.SLTPBufIdx-decisionDelay] = dd.PredQ16[lastIdx]
		}
		NSQ.SLTPShpBufIdx++
		NSQ.SLTPBufIdx++

		// Update path state.
		for k := int32(0); k < nStates; k++ {
			dd := &psDelDec[k]
			ss := &psSampleState[k][0]
			dd.LFARQ12 = ss.LFARQ12
			dd.SLPCQ14[silk.NSQLPCBufLength+i] = ss.XqQ14
			dd.XqQ10[smplBufIdx] = fix.RShift32(ss.XqQ14, 4)
			dd.QQ10[smplBufIdx] = ss.QQ10
			dd.PredQ16[smplBufIdx] = ss.LPCExcQ16
			dd.ShapeQ10[smplBufIdx] = ss.SLTPShpQ10
			dd.Seed = fix.AddRShift32(dd.Seed, ss.QQ10, 10)
			dd.RandState[smplBufIdx] = dd.Seed
			dd.RDQ10 = ss.RDQ10
			dd.GainQ16[smplBufIdx] = gainQ16
		}
	}
	// Slide each path's LPC delay line.
	for k := int32(0); k < nStates; k++ {
		dd := &psDelDec[k]
		copy(dd.SLPCQ14[:silk.NSQLPCBufLength], dd.SLPCQ14[length:length+silk.NSQLPCBufLength])
	}
	return smplBufIdx
}

// nsqDelDecScaleStates — SKP_Silk_nsq_del_dec_scale_states.
//
// Per-path version of nsqScaleStates. The shared NSQ state buffers
// (sLTPShpQ10, the upcoming sLTPQ16 slot) are scaled once; each
// del_dec path's private state (LF_AR, sLPCQ14, sAR2Q14, ring buffers)
// is scaled per-path.
func nsqDelDecScaleStates(NSQ *State, psDelDec []delDecStruct,
	x []int16, xScQ10 []int32, subfrLength int32,
	sLTP []int16, sLTPQ16 []int32, subfr int32,
	nStatesDelayedDecision int32, smplBufIdx int32,
	LTPScaleQ14 int32, gainsQ16 []int32, pitchL []int32) {

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

		for i := NSQ.SLTPShpBufIdx - subfrLength*silk.NBSubFr; i < NSQ.SLTPShpBufIdx; i++ {
			NSQ.SLTPShpQ10[i] = fix.SmulWW(gainAdjQ16, NSQ.SLTPShpQ10[i])
		}
		if NSQ.RewhiteFlag == 0 {
			for i := NSQ.SLTPBufIdx - lag - silk.LTPOrder/2; i < NSQ.SLTPBufIdx; i++ {
				sLTPQ16[i] = fix.SmulWW(gainAdjQ16, sLTPQ16[i])
			}
		}

		for k := int32(0); k < nStatesDelayedDecision; k++ {
			dd := &psDelDec[k]
			dd.LFARQ12 = fix.SmulWW(gainAdjQ16, dd.LFARQ12)
			for i := int32(0); i < silk.NSQLPCBufLength; i++ {
				dd.SLPCQ14[i] = fix.SmulWW(gainAdjQ16, dd.SLPCQ14[i])
			}
			for i := int32(0); i < silk.MaxShapeLPCOrder; i++ {
				dd.SAR2Q14[i] = fix.SmulWW(gainAdjQ16, dd.SAR2Q14[i])
			}
			for i := int32(0); i < silk.DecisionDelay; i++ {
				dd.PredQ16[i] = fix.SmulWW(gainAdjQ16, dd.PredQ16[i])
				dd.ShapeQ10[i] = fix.SmulWW(gainAdjQ16, dd.ShapeQ10[i])
			}
		}
	}

	for i := int32(0); i < subfrLength; i++ {
		xScQ10[i] = fix.RShift32(fix.SmulBB(int32(x[i]), invGainQ16), 6)
	}
	NSQ.PrevInvGainQ16 = invGainQ16
}
