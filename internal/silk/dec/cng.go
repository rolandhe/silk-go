package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/lpc"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
)

// cngExc — file-static SKP_Silk_CNG_exc helper. Generates a CNG residual
// signal of `length` samples by sampling random indices into `excBuf` (Q10)
// and scaling by gainQ16.
func cngExc(residual []int16, excBuf []int32, gainQ16, length int32, randSeed *int32) {
	excMask := int32(silk.CNGBufMaskMax)
	for excMask > length {
		excMask = fix.RShift32(excMask, 1)
	}
	seed := *randSeed
	for i := int32(0); i < length; i++ {
		seed = fix.Rand(seed)
		idx := fix.RShift32(seed, 24) & excMask
		residual[i] = int16(fix.Sat16(fix.RShiftRound(fix.SmulWW(excBuf[idx], gainQ16), 10)))
	}
	*randSeed = seed
}

// CNGReset — SKP_Silk_CNG_Reset.
func CNGReset(psDec *State) {
	step := fix.Div32By16(0x7FFF, psDec.LPCOrder+1)
	acc := int32(0)
	for i := int32(0); i < psDec.LPCOrder; i++ {
		acc += step
		psDec.SCNG.CNGSmthNLSFQ15[i] = acc
	}
	psDec.SCNG.CNGSmthGainQ16 = 0
	psDec.SCNG.RandSeed = 3176576
}

// CNG — SKP_Silk_CNG.
//
// On every call: when speech activity is low (vadFlag == 0) and we're not
// in a packet-loss state, update the smoothed CNG NLSF / gain and refresh
// the excitation buffer with the highest-energy subframe. When the current
// frame is lost, synthesize CNG and mix it into `signal`.
//
// Translated from vendor/silk/src/SKP_Silk_CNG.c.
func CNG(psDec *State, ctrl *Control, signal []int16, length int32) {
	psCNG := &psDec.SCNG

	// Reset on Fs change.
	if psDec.FsKHz != psCNG.FsKHz {
		CNGReset(psDec)
		psCNG.FsKHz = psDec.FsKHz
	}

	if psDec.LossCnt == 0 && psDec.VadFlag == silk.NoVoiceActivity {
		// Smooth NLSFs.
		for i := int32(0); i < psDec.LPCOrder; i++ {
			psCNG.CNGSmthNLSFQ15[i] += fix.SmulWB(
				psDec.PrevNLSFQ15[i]-psCNG.CNGSmthNLSFQ15[i],
				silk.CNGNLSFSmthQ16)
		}

		// Pick highest-gain subframe and copy its excitation into the buffer head.
		var maxGain int32
		var subfr int32
		for i := int32(0); i < silk.NBSubFr; i++ {
			if ctrl.GainsQ16[i] > maxGain {
				maxGain = ctrl.GainsQ16[i]
				subfr = i
			}
		}
		// Shift the buffer right by subfr_length, then copy in the new subframe.
		copy(psCNG.CNGExcBufQ10[psDec.SubfrLength:],
			psCNG.CNGExcBufQ10[:(silk.NBSubFr-1)*psDec.SubfrLength])
		copy(psCNG.CNGExcBufQ10[:psDec.SubfrLength],
			psDec.ExcQ10[subfr*psDec.SubfrLength:(subfr+1)*psDec.SubfrLength])

		// Smooth gains.
		for i := int32(0); i < silk.NBSubFr; i++ {
			psCNG.CNGSmthGainQ16 += fix.SmulWB(
				ctrl.GainsQ16[i]-psCNG.CNGSmthGainQ16,
				silk.CNGGainSmthQ16)
		}
	}

	if psDec.LossCnt != 0 {
		var cngSig [silk.MaxFrameLength]int16
		var lpcBuf [silk.MaxLPCOrder]int16

		cngExc(cngSig[:length], psCNG.CNGExcBufQ10[:],
			psCNG.CNGSmthGainQ16, length, &psCNG.RandSeed)

		// Convert smoothed NLSF to LPC.
		nlsf.NLSF2AStable(lpcBuf[:psDec.LPCOrder], psCNG.CNGSmthNLSFQ15[:psDec.LPCOrder], psDec.LPCOrder)

		gainQ26 := int32(1) << 26
		if psDec.LPCOrder == 16 {
			lpc.SynthesisOrder16(cngSig[:length], lpcBuf[:psDec.LPCOrder], gainQ26,
				psCNG.CNGSynthState[:], cngSig[:length], length)
		} else {
			lpc.SynthesisFilter(cngSig[:length], lpcBuf[:psDec.LPCOrder], gainQ26,
				psCNG.CNGSynthState[:], cngSig[:length], length, psDec.LPCOrder)
		}

		// Mix into the input/output signal.
		for i := int32(0); i < length; i++ {
			signal[i] = int16(fix.Sat16(int32(signal[i]) + int32(cngSig[i])))
		}
	} else {
		for i := int32(0); i < psDec.LPCOrder; i++ {
			psCNG.CNGSynthState[i] = 0
		}
	}
}
