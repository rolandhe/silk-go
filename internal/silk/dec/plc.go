// Package dec — Packet Loss Concealment (PLC).
//
// This file is a Go port of vendor/silk/src/SKP_Silk_PLC.c (SILK
// reference implementation, fixed-point). When a packet is lost the
// decoder synthesises a continuation of the previous frame using the
// last-known LPC, LTP and gain parameters; when a packet arrives the
// PLC state is updated so the next loss has fresh material to work
// with. PLC_glue_frames smooths the energy step at the boundary
// between concealed and recovered frames.
package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/lpc"
)

// Tuning constants from SKP_Silk_PLC.h. Direct ports of the C #defines
// so the concealment behaviour matches the reference bit-for-bit.
const (
	plcBWECoefQ16              int32 = 64880 // 0.99 in Q16
	plcVPitchGainStartMinQ14   int32 = 11469 // 0.7 in Q14
	plcVPitchGainStartMaxQ14   int32 = 15565 // 0.95 in Q14
	plcMaxPitchLagMs           int32 = 18
	plcRandBufSize             int32 = 128
	plcRandBufMask             int32 = plcRandBufSize - 1
	plcLog2InvLPCGainHighThres int32 = 3   // 2^3 = 8 dB LPC gain
	plcLog2InvLPCGainLowThres  int32 = 8   // 2^8 = 24 dB LPC gain
	plcPitchDriftFacQ16        int32 = 655 // 0.01 in Q16
	plcNBAtt                   int32 = 2
)

// Per-loss attenuation tables (NB_ATT entries, indexed by lossCnt
// clamped to NB_ATT-1).
var (
	plcHarmAttQ15         = [...]int16{32440, 31130} // 0.99, 0.95
	plcRandAttenuateVQ15  = [...]int16{31130, 26214} // 0.95, 0.8
	plcRandAttenuateUVQ15 = [...]int16{32440, 29491} // 0.99, 0.9
)

// PLCReset — SKP_Silk_PLC_Reset. Initialises the concealment pitch lag
// to half the frame length so the very first lost frame has a sane
// starting point.
func PLCReset(s *State) {
	s.SPLC.PitchLQ8 = fix.RShift32(s.FrameLength, 1)
}

// PLC — SKP_Silk_PLC. Top-level dispatcher: on a lost frame run the
// concealment synth, otherwise refresh the PLC state from the freshly
// decoded parameters.
func PLC(s *State, ctrl *Control, signal []int16, length int32, lost int32) {
	if s.FsKHz != s.SPLC.FsKHz {
		PLCReset(s)
		s.SPLC.FsKHz = s.FsKHz
	}

	if lost != 0 {
		PLCConceal(s, ctrl, signal, length)
		s.LossCnt++
	} else {
		PLCUpdate(s, ctrl)
	}
}

// PLCUpdate — SKP_Silk_PLC_update. Caches the parameters needed to
// extrapolate from the most recent good frame: dominant LTP coefs and
// pitch lag (voiced) or a synthetic 18 ms lag (unvoiced), the second
// half-frame's LPC, the LTP scale and the per-subframe gains.
func PLCUpdate(s *State, ctrl *Control) {
	psPLC := &s.SPLC

	s.PrevSigtype = ctrl.Sigtype
	var ltpGainQ14 int32

	if ctrl.Sigtype == silk.SigTypeVoiced {
		// Find the parameters for the last subframe which contains a pitch pulse.
		for j := int32(0); j*s.SubfrLength < ctrl.PitchL[silk.NBSubFr-1]; j++ {
			var tempLtpGainQ14 int32
			for i := int32(0); i < silk.LTPOrder; i++ {
				tempLtpGainQ14 += int32(ctrl.LTPCoefQ14[(silk.NBSubFr-1-j)*silk.LTPOrder+i])
			}
			if tempLtpGainQ14 > ltpGainQ14 {
				ltpGainQ14 = tempLtpGainQ14
				base := fix.SmulBB(silk.NBSubFr-1-j, silk.LTPOrder)
				for i := int32(0); i < silk.LTPOrder; i++ {
					psPLC.LTPCoefQ14[i] = ctrl.LTPCoefQ14[base+i]
				}
				psPLC.PitchLQ8 = fix.LShift32(ctrl.PitchL[silk.NBSubFr-1-j], 8)
			}
		}

		// USE_SINGLE_TAP path — the reference code unconditionally
		// collapses the 5-tap filter onto its centre after picking the
		// best subframe.
		for i := int32(0); i < silk.LTPOrder; i++ {
			psPLC.LTPCoefQ14[i] = 0
		}
		psPLC.LTPCoefQ14[silk.LTPOrder/2] = int16(ltpGainQ14)

		// Limit LTP coefficients.
		if ltpGainQ14 < plcVPitchGainStartMinQ14 {
			tmp := fix.LShift32(plcVPitchGainStartMinQ14, 10)
			scaleQ10 := fix.Div32(tmp, fix.Max32(ltpGainQ14, 1))
			for i := int32(0); i < silk.LTPOrder; i++ {
				psPLC.LTPCoefQ14[i] = int16(fix.RShift32(fix.SmulBB(int32(psPLC.LTPCoefQ14[i]), scaleQ10), 10))
			}
		} else if ltpGainQ14 > plcVPitchGainStartMaxQ14 {
			tmp := fix.LShift32(plcVPitchGainStartMaxQ14, 14)
			scaleQ14 := fix.Div32(tmp, fix.Max32(ltpGainQ14, 1))
			for i := int32(0); i < silk.LTPOrder; i++ {
				psPLC.LTPCoefQ14[i] = int16(fix.RShift32(fix.SmulBB(int32(psPLC.LTPCoefQ14[i]), scaleQ14), 14))
			}
		}
	} else {
		psPLC.PitchLQ8 = fix.LShift32(fix.SmulBB(s.FsKHz, 18), 8)
		for i := int32(0); i < silk.LTPOrder; i++ {
			psPLC.LTPCoefQ14[i] = 0
		}
	}

	// Save LPC coefficients (second-half interpolant — index 1).
	for i := int32(0); i < s.LPCOrder; i++ {
		psPLC.PrevLPCQ12[i] = ctrl.PredCoefQ12[1][i]
	}
	psPLC.PrevLTPScaleQ14 = int16(ctrl.LTPScaleQ14)

	// Save gains.
	for i := int32(0); i < silk.NBSubFr; i++ {
		psPLC.PrevGainQ16[i] = ctrl.GainsQ16[i]
	}
}

// PLCConceal — SKP_Silk_PLC_conceal. Synthesises one frame of audio
// that continues the last good frame: scaled-down LTP excitation
// (harmonic part) + scaled random noise from the previous excitation
// buffer, run through the bandwidth-expanded previous LPC.
func PLCConceal(s *State, ctrl *Control, signal []int16, length int32) {
	psPLC := &s.SPLC

	// Update LTP buffer: shift the upper half down.
	copy(s.SLTPQ16[:s.FrameLength], s.SLTPQ16[s.FrameLength:2*s.FrameLength])

	// LPC concealment: bandwidth-expand the saved LPC.
	dsp.BwExpander(psPLC.PrevLPCQ12[:], s.LPCOrder, plcBWECoefQ16)

	// Find random-noise component: scale the previous-frame excitation
	// by its per-subframe gain into a temp buffer.
	var excBuf [silk.MaxFrameLength]int16
	for k := int32(silk.NBSubFr >> 1); k < silk.NBSubFr; k++ {
		dst := (k - (silk.NBSubFr >> 1)) * s.SubfrLength
		for i := int32(0); i < s.SubfrLength; i++ {
			excBuf[dst+i] = int16(fix.RShift32(
				fix.SmulWW(s.ExcQ10[i+k*s.SubfrLength], psPLC.PrevGainQ16[k]), 10))
		}
	}

	// Pick the lower-energy of the last two subframes as the noise
	// source; cross-multiply by each other's shift to compare without
	// overflow.
	energy1, shift1 := dsp.SumSqrShift(excBuf[:s.SubfrLength], s.SubfrLength)
	energy2, shift2 := dsp.SumSqrShift(excBuf[s.SubfrLength:2*s.SubfrLength], s.SubfrLength)

	var randPtrOff int32
	if fix.RShift32(energy1, shift2) < fix.RShift32(energy2, shift1) {
		// First sub-frame has lowest energy.
		randPtrOff = fix.MaxInt(0, 3*s.SubfrLength-plcRandBufSize)
	} else {
		// Second sub-frame has lowest energy.
		randPtrOff = fix.MaxInt(0, s.FrameLength-plcRandBufSize)
	}

	BQ14 := psPLC.LTPCoefQ14[:]
	randScaleQ14 := psPLC.RandScaleQ14

	// Per-loss-count attenuation gains.
	attIdx := fix.MinInt(plcNBAtt-1, s.LossCnt)
	harmGainQ15 := int32(plcHarmAttQ15[attIdx])
	var randGainQ15 int32
	if s.PrevSigtype == silk.SigTypeVoiced {
		randGainQ15 = int32(plcRandAttenuateVQ15[attIdx])
	} else {
		randGainQ15 = int32(plcRandAttenuateUVQ15[attIdx])
	}

	// First lost frame: derive randScaleQ14 from the current-frame
	// LTP/LPC instead of inheriting it from the previous lost frame.
	if s.LossCnt == 0 {
		randScaleQ14 = int16(1) << 14

		if s.PrevSigtype == silk.SigTypeVoiced {
			// C does each subtraction in int16 (rand_scale_Q14 and
			// B_Q14[i] are both SKP_int16) with two's-complement
			// truncation between iterations. Mirror that exactly so
			// degenerate LTP coefficient vectors that overflow the
			// int16 path produce the same wrapped value as C.
			for i := int32(0); i < silk.LTPOrder; i++ {
				randScaleQ14 = int16(int32(randScaleQ14) - int32(BQ14[i]))
			}
			if randScaleQ14 < 3277 {
				randScaleQ14 = 3277 // 0.2 in Q14
			}
			randScaleQ14 = int16(fix.RShift32(
				fix.SmulBB(int32(randScaleQ14), int32(psPLC.PrevLTPScaleQ14)), 14))
		}

		// Reduce random noise for unvoiced frames with high LPC gain.
		if s.PrevSigtype == silk.SigTypeUnvoiced {
			var invGainQ30 int32
			lpc.LPCInversePredGain(&invGainQ30, psPLC.PrevLPCQ12[:], s.LPCOrder)

			downScaleQ30 := fix.Min32(fix.RShift32(int32(1)<<30, plcLog2InvLPCGainHighThres), invGainQ30)
			downScaleQ30 = fix.Max32(fix.RShift32(int32(1)<<30, plcLog2InvLPCGainLowThres), downScaleQ30)
			downScaleQ30 = fix.LShift32(downScaleQ30, plcLog2InvLPCGainHighThres)

			randGainQ15 = fix.RShift32(fix.SmulWB(downScaleQ30, randGainQ15), 14)
		}
	}

	randSeed := psPLC.RandSeed
	lag := fix.RShiftRound(psPLC.PitchLQ8, 8)
	sLTPBufIdx := s.FrameLength

	// LTP synthesis filtering: produce sigQ10 = harmonic + noise for
	// the whole frame, advancing the LTP buffer one sample at a time.
	var sigQ10 [silk.MaxFrameLength]int32

	for k := int32(0); k < silk.NBSubFr; k++ {
		// Pointer to the lag-shifted history (with LTP/2 lookahead).
		predLagBase := sLTPBufIdx - lag + silk.LTPOrder/2
		sigOff := k * s.SubfrLength

		for i := int32(0); i < s.SubfrLength; i++ {
			randSeed = fix.Rand(randSeed)
			idx := fix.RShift32(randSeed, 25) & plcRandBufMask

			// 5-tap LTP prediction (centre tap is index 0 here).
			ltpPredQ14 := fix.SmulWB(s.SLTPQ16[predLagBase+i+0], int32(BQ14[0]))
			ltpPredQ14 = fix.SmlaWB(ltpPredQ14, s.SLTPQ16[predLagBase+i-1], int32(BQ14[1]))
			ltpPredQ14 = fix.SmlaWB(ltpPredQ14, s.SLTPQ16[predLagBase+i-2], int32(BQ14[2]))
			ltpPredQ14 = fix.SmlaWB(ltpPredQ14, s.SLTPQ16[predLagBase+i-3], int32(BQ14[3]))
			ltpPredQ14 = fix.SmlaWB(ltpPredQ14, s.SLTPQ16[predLagBase+i-4], int32(BQ14[4]))

			// Generate LPC residual: noise (Q10) + harmonic (Q14→Q10).
			lpcExcQ10 := fix.LShift32(fix.SmulWB(s.ExcQ10[randPtrOff+idx], int32(randScaleQ14)), 2)
			lpcExcQ10 = fix.Add32(lpcExcQ10, fix.RShiftRound(ltpPredQ14, 4))

			// Update LTP buffer state and stash the residual.
			s.SLTPQ16[sLTPBufIdx] = fix.LShift32(lpcExcQ10, 6)
			sLTPBufIdx++

			sigQ10[sigOff+i] = lpcExcQ10
		}

		// Gradually reduce LTP gain.
		for j := int32(0); j < silk.LTPOrder; j++ {
			BQ14[j] = int16(fix.RShift32(fix.SmulBB(harmGainQ15, int32(BQ14[j])), 15))
		}
		// Gradually reduce excitation gain.
		randScaleQ14 = int16(fix.RShift32(fix.SmulBB(int32(randScaleQ14), randGainQ15), 15))

		// Slowly increase pitch lag.
		psPLC.PitchLQ8 += fix.SmulWB(psPLC.PitchLQ8, plcPitchDriftFacQ16)
		psPLC.PitchLQ8 = fix.Min32(psPLC.PitchLQ8, fix.LShift32(fix.SmulBB(plcMaxPitchLagMs, s.FsKHz), 8))
		lag = fix.RShiftRound(psPLC.PitchLQ8, 8)
	}

	// LPC synthesis filtering — direct port of the unrolled per-sample
	// loop. The C source has a packed LE int32 path that loads two
	// int16 coefs per word; the equivalent unpacked SmlaWB sequence is
	// bit-identical and matches what decode_short_term_prediction does.
	for k := int32(0); k < silk.NBSubFr; k++ {
		sigOff := k * s.SubfrLength
		for i := int32(0); i < s.SubfrLength; i++ {
			var lpcPredQ10 int32
			for j := int32(0); j < s.LPCOrder; j++ {
				lpcPredQ10 = fix.SmlaWB(lpcPredQ10,
					s.SLPCQ14[silk.MaxLPCOrder+i-j-1],
					int32(psPLC.PrevLPCQ12[j]))
			}

			// Add prediction to LPC residual.
			sigQ10[sigOff+i] = fix.Add32(sigQ10[sigOff+i], lpcPredQ10)

			// Update LPC state.
			s.SLPCQ14[silk.MaxLPCOrder+i] = fix.LShift32(sigQ10[sigOff+i], 4)
		}
		// Shift LPC filter state down for the next subframe.
		copy(s.SLPCQ14[:silk.MaxLPCOrder], s.SLPCQ14[s.SubfrLength:s.SubfrLength+silk.MaxLPCOrder])
	}

	// Scale with gain (last subframe gain) and saturate to int16.
	gain := psPLC.PrevGainQ16[silk.NBSubFr-1]
	for i := int32(0); i < s.FrameLength; i++ {
		signal[i] = int16(fix.Sat16(fix.RShiftRound(fix.SmulWW(sigQ10[i], gain), 10)))
	}

	// Persist updated PLC state.
	psPLC.RandSeed = randSeed
	psPLC.RandScaleQ14 = randScaleQ14
	for i := int32(0); i < silk.NBSubFr; i++ {
		ctrl.PitchL[i] = lag
	}
}

// PLCGlueFrames — SKP_Silk_PLC_glue_frames. Smooths the energy step at
// the boundary between concealed output and the first recovered good
// frame: caches concealed energy on each lost frame; on the first good
// frame after a loss, fades from sqrt(conc/curr) gain back to 1.
func PLCGlueFrames(s *State, ctrl *Control, signal []int16, length int32) {
	psPLC := &s.SPLC

	if s.LossCnt != 0 {
		// Cache energy of the (concealed) residual.
		psPLC.ConcEnergy, psPLC.ConcEnergyShift = dsp.SumSqrShift(signal, length)
		psPLC.LastFrameLost = 1
	} else {
		if psPLC.LastFrameLost != 0 {
			energy, energyShift := dsp.SumSqrShift(signal, length)

			// Normalise energies to a common Q.
			if energyShift > psPLC.ConcEnergyShift {
				psPLC.ConcEnergy = fix.RShift32(psPLC.ConcEnergy, energyShift-psPLC.ConcEnergyShift)
			} else if energyShift < psPLC.ConcEnergyShift {
				energy = fix.RShift32(energy, psPLC.ConcEnergyShift-energyShift)
			}

			// Fade in the energy difference if current > concealed.
			if energy > psPLC.ConcEnergy {
				lz := fix.Clz32(psPLC.ConcEnergy) - 1
				psPLC.ConcEnergy = fix.LShift32(psPLC.ConcEnergy, lz)
				energy = fix.RShift32(energy, fix.Max32(24-lz, 0))

				fracQ24 := fix.Div32(psPLC.ConcEnergy, fix.Max32(energy, 1))

				gainQ12 := fix.SqrtApprox(fracQ24)
				slopeQ12 := fix.Div32By16((int32(1)<<12)-gainQ12, length)

				for i := int32(0); i < length; i++ {
					signal[i] = int16(fix.RShift32(fix.Mul(gainQ12, int32(signal[i])), 12))
					gainQ12 += slopeQ12
					gainQ12 = fix.Min32(gainQ12, int32(1)<<12)
				}
			}
		}
		psPLC.LastFrameLost = 0
	}
}
