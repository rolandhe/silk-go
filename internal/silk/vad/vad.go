// Package vad implements SILK's voice-activity detector.
//
// The VAD splits the input into four logarithmic frequency bands using a
// cascade of half-band analysis filters, tracks the per-band noise floor,
// and turns the resulting per-band SNR into a speech-activity probability.
//
// Translated from vendor/silk/src/SKP_Silk_VAD.c.
package vad

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

const (
	int16Max int32 = 32767
	int32Max int32 = 0x7FFFFFFF
	uint8Max int32 = 255
)

// State mirrors SKP_Silk_VAD_state. Public fields keep the C names (in
// Go-idiomatic case) so encoder code that pokes at the state directly is
// straightforward to translate.
type State struct {
	AnaState       [2]int32              // analysis filter-bank state, band 0
	AnaState1      [2]int32              // analysis filter-bank state, band 1
	AnaState2      [2]int32              // analysis filter-bank state, band 2
	XnrgSubfr      [silk.VADNBands]int32 // subframe energy carry-over
	NrgRatioSmthQ8 [silk.VADNBands]int32 // smoothed energy-to-noise ratio (Q8)
	HPstate        int16                 // single-tap HP differentiator state
	NL             [silk.VADNBands]int32 // current noise level estimates
	InvNL          [silk.VADNBands]int32 // smoothed inverse noise levels
	NoiseLevelBias [silk.VADNBands]int32 // per-band bias toward pink noise
	Counter        int32                 // frames seen since reset
}

// Init — SKP_Silk_VAD_Init.
//
// Resets the state and seeds the noise estimate to a roughly pink spectrum so
// initial frames don't all look like clean speech.
func Init(s *State) int32 {
	*s = State{}

	for b := int32(0); b < silk.VADNBands; b++ {
		s.NoiseLevelBias[b] = fix.Max32(silk.VADNoiseLevelsBias/(b+1), 1)
	}
	for b := int32(0); b < silk.VADNBands; b++ {
		s.NL[b] = 100 * s.NoiseLevelBias[b]
		s.InvNL[b] = int32Max / s.NL[b]
	}
	s.Counter = 15
	for b := int32(0); b < silk.VADNBands; b++ {
		s.NrgRatioSmthQ8[b] = 100 * 256 // ≈ 20 dB SNR
	}
	return 0
}

// tiltWeights — frequency-tilt weighting per band (low → high).
var tiltWeights = [silk.VADNBands]int32{30000, 6000, -12000, -12000}

// GetSAQ8 — SKP_Silk_VAD_GetSA_Q8.
//
// Runs one frame through the VAD. Writes:
//   - pSAQ8       speech-activity level in Q8 (0..255)
//   - pSNRdBQ7    overall SNR estimate in Q7 dB
//   - pQualityQ15 per-band smoothed-SNR sigmoid in Q15
//   - pTiltQ15    spectral-tilt sigmoid in Q15 (centered at 0)
//
// Returns 0 (the C source never returns non-zero either).
func GetSAQ8(
	s *State,
	pSAQ8, pSNRdBQ7, pTiltQ15 *int32,
	pQualityQ15 []int32,
	pIn []int16,
	frameLength int32,
) int32 {
	// Per-band decimated samples — at most MaxFrameLength/2 each.
	var X [silk.VADNBands][silk.MaxFrameLength / 2]int16
	var Xnrg [silk.VADNBands]int32
	var NrgToNoiseRatioQ8 [silk.VADNBands]int32

	// 0–8 kHz → 0–4 / 4–8 kHz.
	dsp.AnaFiltBank1(pIn, s.AnaState[:], X[0][:], X[3][:], frameLength)
	// 0–4 kHz → 0–2 / 2–4 kHz.
	dsp.AnaFiltBank1(X[0][:], s.AnaState1[:], X[0][:], X[2][:], fix.RShift32(frameLength, 1))
	// 0–2 kHz → 0–1 / 1–2 kHz.
	dsp.AnaFiltBank1(X[0][:], s.AnaState2[:], X[0][:], X[1][:], fix.RShift32(frameLength, 2))

	// HP differentiator on lowest band — mirrors the awkward in-place
	// loop from the C source (overwrites X[0] back-to-front).
	decFL := fix.RShift32(frameLength, 3)
	X[0][decFL-1] >>= 1
	hpStateTmp := X[0][decFL-1]
	for i := decFL - 1; i > 0; i-- {
		X[0][i-1] >>= 1
		X[0][i] -= X[0][i-1]
	}
	X[0][0] -= s.HPstate
	s.HPstate = hpStateTmp

	// Per-band energy across VAD_INTERNAL_SUBFRAMES sub-frames.
	var sumSquared int32
	for b := int32(0); b < silk.VADNBands; b++ {
		shift := silk.VADNBands - b
		if shift > silk.VADNBands-1 {
			shift = silk.VADNBands - 1
		}
		decimatedFL := fix.RShift32(frameLength, shift)
		decSubFL := fix.RShift32(decimatedFL, silk.VADInternalSubframesLog2)
		decSubOff := int32(0)

		// Initialize with carry-over from the previous frame's last
		// subframe.
		Xnrg[b] = s.XnrgSubfr[b]
		var lastSum int32
		for sub := int32(0); sub < silk.VADInternalSubframes; sub++ {
			ss := int32(0)
			for i := int32(0); i < decSubFL; i++ {
				v := fix.RShift32(int32(X[b][i+decSubOff]), 3)
				ss = fix.SmlaBB(ss, v, v)
			}
			if sub < silk.VADInternalSubframes-1 {
				Xnrg[b] = fix.AddPosSat32(Xnrg[b], ss)
			} else {
				// Look-ahead subframe contributes half.
				Xnrg[b] = fix.AddPosSat32(Xnrg[b], fix.RShift32(ss, 1))
			}
			lastSum = ss
			decSubOff += decSubFL
		}
		s.XnrgSubfr[b] = lastSum
	}

	// Track noise floor per band.
	GetNoiseLevels(Xnrg[:], s)

	// Per-band SNR → squared-sum + tilt accumulator.
	sumSquared = 0
	inputTilt := int32(0)
	for b := int32(0); b < silk.VADNBands; b++ {
		speechNrg := Xnrg[b] - s.NL[b]
		if speechNrg > 0 {
			// Pick scaling that keeps the divide in 32-bit range.
			if uint32(Xnrg[b])&0xFF800000 == 0 {
				NrgToNoiseRatioQ8[b] = fix.LShift32(Xnrg[b], 8) / (s.NL[b] + 1)
			} else {
				NrgToNoiseRatioQ8[b] = Xnrg[b] / (fix.RShift32(s.NL[b], 8) + 1)
			}

			snrQ7 := dsp.Lin2Log(NrgToNoiseRatioQ8[b]) - 8*128
			sumSquared = fix.SmlaBB(sumSquared, snrQ7, snrQ7)

			// For very small subband speech energies, scale down the
			// SNR so it doesn't dominate the tilt measure.
			if speechNrg < (1 << 20) {
				snrQ7 = fix.SmulWB(fix.LShift32(fix.SqrtApprox(speechNrg), 6), snrQ7)
			}
			inputTilt = fix.SmlaWB(inputTilt, tiltWeights[b], snrQ7)
		} else {
			NrgToNoiseRatioQ8[b] = 256
		}
	}

	// Mean-of-squares → RMS-approx → 3 × value (≈ dB scaling). Q14 → Q7.
	sumSquared = sumSquared / silk.VADNBands
	*pSNRdBQ7 = int32(int16(3 * fix.SqrtApprox(sumSquared)))

	// Speech probability via sigmoid on Q5 input. SmulWB(SNR_factor, dB)
	// is Q5 (Q16 * Q7 >> 16 = Q7, minus Q5 offset folds into shift).
	saQ15 := dsp.SigmQ15(fix.SmulWB(silk.VADSNRFactorQ16, *pSNRdBQ7) - silk.VADNegativeOffsetQ5)

	// Frequency-tilt sigmoid, recentered around zero (the C code shifts a
	// signed Q15 sigmoid by half the range and left-shifts by 1).
	*pTiltQ15 = fix.LShift32(dsp.SigmQ15(inputTilt)-16384, 1)

	// Power-based scaling: weight bands by index, then scale SA by sqrt of
	// the resulting sum if it's small.
	speechNrg := int32(0)
	for b := int32(0); b < silk.VADNBands; b++ {
		speechNrg += (b + 1) * fix.RShift32(Xnrg[b]-s.NL[b], 4)
	}
	if speechNrg <= 0 {
		saQ15 = fix.RShift32(saQ15, 1)
	} else if speechNrg < 32768 {
		speechNrg = fix.SqrtApprox(fix.LShift32(speechNrg, 15))
		saQ15 = fix.SmulWB(32768+speechNrg, saQ15)
	}

	*pSAQ8 = fix.MinInt(fix.RShift32(saQ15, 7), uint8Max)

	// Smooth per-band SNR for the encoder's quality output.
	smoothCoefQ16 := fix.SmulWB(silk.VADSNRSmoothCoefQ18, fix.SmulWB(saQ15, saQ15))
	for b := int32(0); b < silk.VADNBands; b++ {
		s.NrgRatioSmthQ8[b] = fix.SmlaWB(s.NrgRatioSmthQ8[b],
			NrgToNoiseRatioQ8[b]-s.NrgRatioSmthQ8[b], smoothCoefQ16)
		snrQ7 := 3 * (dsp.Lin2Log(s.NrgRatioSmthQ8[b]) - 8*128)
		// quality ≈ sigmoid( 0.25 * (SNR_dB - 16) )
		pQualityQ15[b] = dsp.SigmQ15(fix.RShift32(snrQ7-16*128, 4))
	}

	return 0
}

// GetNoiseLevels — SKP_Silk_VAD_GetNoiseLevels.
//
// Updates each band's noise-level estimate from its current subframe energy
// using a leaky inverse-energy smoother. Smoothing is faster for the first
// ~20 seconds of audio so the floor catches up quickly after init.
func GetNoiseLevels(pX []int32, s *State) {
	var minCoef int32
	if s.Counter < 1000 {
		minCoef = int16Max / (fix.RShift32(s.Counter, 4) + 1)
	}

	for k := int32(0); k < silk.VADNBands; k++ {
		nl := s.NL[k]

		// Bias toward pink noise — keeps the filter from collapsing on
		// silence and gives the next divide a non-zero denominator.
		nrg := fix.AddPosSat32(pX[k], s.NoiseLevelBias[k])
		invNrg := int32Max / nrg

		var coef int32
		switch {
		case nrg > fix.LShift32(nl, 3):
			// Strong band → very slow update (presumed speech).
			coef = silk.VADNoiseLevelSmoothCoefQ16 >> 3
		case nrg < nl:
			// Below current floor → full update.
			coef = silk.VADNoiseLevelSmoothCoefQ16
		default:
			// Linear ramp between the two extremes.
			coef = fix.SmulWB(fix.SmulWW(invNrg, nl), silk.VADNoiseLevelSmoothCoefQ16<<1)
		}
		coef = fix.MaxInt(coef, minCoef)

		s.InvNL[k] = fix.SmlaWB(s.InvNL[k], invNrg-s.InvNL[k], coef)

		// Re-invert with 7-bit headroom guard.
		nl = int32Max / s.InvNL[k]
		if nl > 0x00FFFFFF {
			nl = 0x00FFFFFF
		}
		s.NL[k] = nl
	}

	s.Counter++
}
