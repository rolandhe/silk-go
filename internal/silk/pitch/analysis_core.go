// Package pitch implements SILK's open-loop pitch estimator (the FIXED-POINT
// pitch_analysis_core + its two scratch helpers). The encoder calls
// AnalysisCore once per frame with the LPC-residual signal; the result is a
// voicing flag plus four per-subframe lag values and the codebook indices
// needed to entropy-code them.
//
// Translated from vendor/silk/src/SKP_Silk_pitch_analysis_core.c.
package pitch

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/resampler"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// scratchSize matches SCRATCH_SIZE from the C source.
const scratchSize = 22

// findScaling — SKP_FIX_P_Ana_find_scaling.
//
// Compute a right-shift count that keeps a windowed inner product from
// overflowing int32. Considers both the worst-case sample (max-abs) and
// the window length.
func findScaling(signal []int16, signalLength, sumSqrLen int32) int32 {
	xMax := int32(dsp.Int16ArrayMaxAbs(signal, signalLength))

	var nbits int32
	if xMax < 32767 {
		nbits = 32 - fix.Clz32(fix.SmulBB(xMax, xMax))
	} else {
		// Saturated max — assume worst case.
		nbits = 30
	}
	nbits += 17 - fix.Clz16(int16(sumSqrLen))
	if nbits < 31 {
		return 0
	}
	return nbits - 30
}

// calcCorrSt3 — SKP_FIX_P_Ana_calc_corr_st3.
//
// Stage-3 cross-correlation table: for each subframe k and each codebook
// vector j (within the active range), records the cross-correlation of
// the target with the basis at start_lag + CB_lags_stage3[k][j] for each
// of NB_STAGE3_LAGS lag offsets. The recursive scratch indexing
// guarantees we don't recompute correlations as `i` walks through the
// codebook.
func calcCorrSt3(
	crossCorrSt3 *[silk.PitchEstNBSubFr][silk.PitchEstNBCbksStage3Max][silk.PitchEstNBStage3Lags]int32,
	signal []int16,
	startLag, sfLength, complexity int32,
) {
	cbkOffset := int32(tables.Cbk_offsets_stage3[complexity])
	cbkSize := int32(tables.Cbk_sizes_stage3[complexity])

	var scratchMem [scratchSize]int32
	targetOff := fix.LShift32(sfLength, 2) // middle of frame

	for k := int32(0); k < silk.PitchEstNBSubFr; k++ {
		lo := int32(tables.Lag_range_stage3[complexity][k][0])
		hi := int32(tables.Lag_range_stage3[complexity][k][1])

		// Pre-compute correlations across the lag range.
		lagCounter := int32(0)
		for j := lo; j <= hi; j++ {
			basisOff := targetOff - (startLag + j)
			scratchMem[lagCounter] = dsp.InnerProdAligned(
				signal[targetOff:], signal[basisOff:], sfLength)
			lagCounter++
		}

		// Fan out into the 3D table indexed by codebook vector.
		delta := lo
		for i := cbkOffset; i < cbkOffset+cbkSize; i++ {
			idx := int32(tables.CB_lags_stage3[k][i]) - delta
			for j := int32(0); j < silk.PitchEstNBStage3Lags; j++ {
				crossCorrSt3[k][i][j] = scratchMem[idx+j]
			}
		}
		targetOff += sfLength
	}
}

// calcEnergySt3 — SKP_FIX_P_Ana_calc_energy_st3.
//
// Stage-3 energy table. Sliding-window recursion: the energy at the next
// lag drops the trailing sample's square and adds the new leading
// sample's square.
func calcEnergySt3(
	energiesSt3 *[silk.PitchEstNBSubFr][silk.PitchEstNBCbksStage3Max][silk.PitchEstNBStage3Lags]int32,
	signal []int16,
	startLag, sfLength, complexity int32,
) {
	cbkOffset := int32(tables.Cbk_offsets_stage3[complexity])
	cbkSize := int32(tables.Cbk_sizes_stage3[complexity])

	var scratchMem [scratchSize]int32
	targetOff := fix.LShift32(sfLength, 2)

	for k := int32(0); k < silk.PitchEstNBSubFr; k++ {
		lo := int32(tables.Lag_range_stage3[complexity][k][0])
		hi := int32(tables.Lag_range_stage3[complexity][k][1])

		lagCounter := int32(0)
		basisOff := targetOff - (startLag + lo)
		energy := dsp.InnerProdAligned(signal[basisOff:], signal[basisOff:], sfLength)
		scratchMem[lagCounter] = energy
		lagCounter++

		for i := int32(1); i < hi-lo+1; i++ {
			// Drop trailing sample, add new leading one. basisOff is the
			// pointer at i=0; for the i-th step the trailing index is
			// (basisOff + sfLength - i) and the new leading index is
			// (basisOff - i).
			trail := int32(signal[basisOff+sfLength-i])
			energy -= fix.SmulBB(trail, trail)
			lead := int32(signal[basisOff-i])
			energy = fix.AddSat32(energy, fix.SmulBB(lead, lead))
			scratchMem[lagCounter] = energy
			lagCounter++
		}

		delta := lo
		for i := cbkOffset; i < cbkOffset+cbkSize; i++ {
			idx := int32(tables.CB_lags_stage3[k][i]) - delta
			for j := int32(0); j < silk.PitchEstNBStage3Lags; j++ {
				energiesSt3[k][i][j] = scratchMem[idx+j]
			}
		}
		targetOff += sfLength
	}
}

// AnalysisCore — SKP_Silk_pitch_analysis_core.
//
// Multi-stage open-loop pitch estimator:
//   - Decimate to 4 kHz (for cheap initial search).
//   - Stage 1: full-range correlation search at 4 kHz, sort, threshold.
//   - Stage 2: refine candidate lags at 8 kHz against codebook contours.
//   - Stage 3: refine again at the input rate (only when Fs > 8 kHz).
//
// Returns 0 (voiced) on success, 1 (unvoiced) when the correlation peaks
// fall under the threshold. On unvoiced the output `pitchOut` is zeroed
// and the indices are 0.
//
// Translated from vendor/silk/src/SKP_Silk_pitch_analysis_core.c.
func AnalysisCore(
	signal []int16,
	pitchOut []int32,
	lagIndex *int32,
	contourIndex *int32,
	LTPCorrQ15 *int32,
	prevLag int32,
	searchThres1Q16 int32,
	searchThres2Q15 int32,
	FsKHz int32,
	complexity int32,
	forLJC int32,
) int32 {
	// Setup frame lengths / lag bounds for this Fs.
	frameLength := silk.PitchEstFrameLengthMs * FsKHz
	frameLength4kHz := int32(silk.PitchEstFrameLengthMs * 4)
	frameLength8kHz := int32(silk.PitchEstFrameLengthMs * 8)
	sfLength := frameLength >> 3
	sfLength8kHz := frameLength8kHz >> 3
	minLag := silk.PitchEstMinLagMs * FsKHz
	minLag4kHz := int32(silk.PitchEstMinLagMs * 4)
	minLag8kHz := int32(silk.PitchEstMinLagMs * 8)
	maxLag := silk.PitchEstMaxLagMs * FsKHz
	maxLag4kHz := int32(silk.PitchEstMaxLagMs * 4)
	maxLag8kHz := int32(silk.PitchEstMaxLagMs * 8)

	var signal8kHz [silk.PitchEstMaxFrameLengthSt2]int16
	var signal4kHz [silk.PitchEstMaxFrameLengthSt1]int16
	var scratchMem [3 * silk.PitchEstMaxFrameLength]int32 // also used as int16 buffer

	var C [silk.PitchEstNBSubFr][(silk.PitchEstMaxLag >> 1) + 5]int16

	// Step 1a: decimate to 8 kHz.
	switch FsKHz {
	case 16:
		var st [2]int32
		resampler.Down2(st[:], signal8kHz[:], signal, frameLength)
	case 12:
		var R23 [6]int32
		resampler.Down23(R23[:], signal8kHz[:], signal, silk.PitchEstFrameLengthMs*12)
	case 24:
		var st [8]int32
		resampler.Down3(st[:], signal8kHz[:], signal, 24*silk.PitchEstFrameLengthMs)
	default: // 8
		copy(signal8kHz[:frameLength8kHz], signal[:frameLength8kHz])
	}

	// Step 1b: decimate again to 4 kHz, then a one-tap LP filter.
	var filtState [2]int32
	resampler.Down2(filtState[:], signal4kHz[:], signal8kHz[:], frameLength8kHz)
	for i := frameLength4kHz - 1; i > 0; i-- {
		signal4kHz[i] = int16(fix.Sat16(int32(signal4kHz[i]) + int32(signal4kHz[i-1])))
	}

	// Scale 4 kHz signal down so inner products can't overflow.
	maxSumSqLength := fix.MaxInt(sfLength8kHz, frameLength4kHz>>1)
	shift := findScaling(signal4kHz[:], frameLength4kHz, maxSumSqLength)
	if shift > 0 {
		for i := int32(0); i < frameLength4kHz; i++ {
			signal4kHz[i] = int16(fix.RShift32(int32(signal4kHz[i]), shift))
		}
	}

	// Stage 1: full-range correlation search at 4 kHz.
	targetOff := frameLength4kHz >> 1
	for k := int32(0); k < 2; k++ {
		basisOff := targetOff - minLag4kHz

		crossCorr := dsp.InnerProdAligned(signal4kHz[targetOff:], signal4kHz[basisOff:], sfLength8kHz)
		normalizer := dsp.InnerProdAligned(signal4kHz[basisOff:], signal4kHz[basisOff:], sfLength8kHz)
		normalizer = fix.AddSat32(normalizer, fix.SmulBB(sfLength8kHz, 4000))

		temp := crossCorr / (fix.SqrtApprox(normalizer) + 1)
		C[k][minLag4kHz] = int16(fix.Sat16(temp))

		for d := minLag4kHz + 1; d <= maxLag4kHz; d++ {
			basisOff--
			crossCorr = dsp.InnerProdAligned(signal4kHz[targetOff:], signal4kHz[basisOff:], sfLength8kHz)
			// Recursive normalizer update.
			normalizer += fix.SmulBB(int32(signal4kHz[basisOff]), int32(signal4kHz[basisOff])) -
				fix.SmulBB(int32(signal4kHz[basisOff+sfLength8kHz]), int32(signal4kHz[basisOff+sfLength8kHz]))
			temp = crossCorr / (fix.SqrtApprox(normalizer) + 1)
			C[k][d] = int16(fix.Sat16(temp))
		}
		targetOff += sfLength8kHz
	}

	// Combine the two subframes' correlations and apply short-lag bias.
	for i := maxLag4kHz; i >= minLag4kHz; i-- {
		sum := int32(C[0][i]) + int32(C[1][i])
		sum = fix.RShift32(sum, 1)
		sum = fix.SmlaWB(sum, sum, fix.LShift32(-i, 4))
		C[0][i] = int16(sum)
	}

	// Sort top-N candidates.
	lengthDSrch := 4 + 2*complexity
	var dSrch [silk.PitchEstDSrchLength]int32
	dsp.InsertionSortDecreasingInt16(C[0][minLag4kHz:], dSrch[:],
		maxLag4kHz-minLag4kHz+1, lengthDSrch)

	// Escape hatch: if even the best correlation is much lower than the
	// energy, declare unvoiced.
	targetOff = frameLength4kHz >> 1
	energy := dsp.InnerProdAligned(signal4kHz[targetOff:], signal4kHz[targetOff:], frameLength4kHz>>1)
	energy = fix.AddPosSat32(energy, 1000)
	Cmax := int32(C[0][minLag4kHz])
	threshold := fix.SmulBB(Cmax, Cmax)
	if fix.RShift32(energy, 4+2) > threshold {
		for i := range pitchOut[:silk.PitchEstNBSubFr] {
			pitchOut[i] = 0
		}
		*LTPCorrQ15 = 0
		*lagIndex = 0
		*contourIndex = 0
		return 1
	}

	// Threshold candidate set, then convert to 8 kHz indices.
	threshold = fix.SmulWB(searchThres1Q16, Cmax)
	for i := int32(0); i < lengthDSrch; i++ {
		if int32(C[0][minLag4kHz+i]) > threshold {
			dSrch[i] = (dSrch[i] + minLag4kHz) << 1
		} else {
			lengthDSrch = i
			break
		}
	}

	// Build dilation map of allowed 8 kHz lags via two convolutions.
	var dComp [(silk.PitchEstMaxLag >> 1) + 5]int16
	for i := minLag8kHz - 5; i < maxLag8kHz+5; i++ {
		dComp[i] = 0
	}
	for i := int32(0); i < lengthDSrch; i++ {
		dComp[dSrch[i]] = 1
	}
	for i := maxLag8kHz + 3; i >= minLag8kHz; i-- {
		dComp[i] += dComp[i-1] + dComp[i-2]
	}

	lengthDSrch = 0
	for i := minLag8kHz; i < maxLag8kHz+1; i++ {
		if dComp[i+1] > 0 {
			dSrch[lengthDSrch] = i
			lengthDSrch++
		}
	}
	for i := maxLag8kHz + 3; i >= minLag8kHz; i-- {
		dComp[i] += dComp[i-1] + dComp[i-2] + dComp[i-3]
	}
	lengthDComp := int32(0)
	for i := minLag8kHz; i < maxLag8kHz+4; i++ {
		if dComp[i] > 0 {
			dComp[lengthDComp] = int16(i - 2)
			lengthDComp++
		}
	}

	// Stage 2: refine at 8 kHz.
	shift = findScaling(signal8kHz[:], frameLength8kHz, sfLength8kHz)
	if shift > 0 {
		for i := int32(0); i < frameLength8kHz; i++ {
			signal8kHz[i] = int16(fix.RShift32(int32(signal8kHz[i]), shift))
		}
	}

	for k := range C {
		for i := range C[k] {
			C[k][i] = 0
		}
	}
	targetOff = frameLength4kHz // middle of 8 kHz frame
	for k := int32(0); k < silk.PitchEstNBSubFr; k++ {
		energyTarget := dsp.InnerProdAligned(signal8kHz[targetOff:], signal8kHz[targetOff:], sfLength8kHz)
		for j := int32(0); j < lengthDComp; j++ {
			d := int32(dComp[j])
			basisOff := targetOff - d
			crossCorr := dsp.InnerProdAligned(signal8kHz[targetOff:], signal8kHz[basisOff:], sfLength8kHz)
			energyBasis := dsp.InnerProdAligned(signal8kHz[basisOff:], signal8kHz[basisOff:], sfLength8kHz)
			if crossCorr > 0 {
				e := fix.MaxInt(energyTarget, energyBasis)
				lz := fix.Clz32(crossCorr)
				lshift := fix.Limit(lz-1, 0, 15)
				temp := fix.LShift32(crossCorr, lshift) / (fix.RShift32(e, 15-lshift) + 1) // Q15
				temp = fix.SmulWB(crossCorr, temp)                                         // Q-1
				temp = fix.AddSat32(temp, temp)                                            // Q0
				lz = fix.Clz32(temp)
				lshift = fix.Limit(lz-1, 0, 15)
				e = fix.MinInt(energyTarget, energyBasis)
				C[k][d] = int16(fix.LShift32(temp, lshift) / (fix.RShift32(e, 15-lshift) + 1))
			}
		}
		targetOff += sfLength8kHz
	}

	// Search lag range × stage-2 codebook for the best biased CC.
	CCmax := int32(-1 << 31)
	CCmaxB := int32(-1 << 31)
	CBimax := int32(0)
	lag := int32(-1)

	prevLagAdj := prevLag
	var prevLagLog2Q7 int32
	if prevLagAdj > 0 {
		switch FsKHz {
		case 12:
			prevLagAdj = fix.LShift32(prevLagAdj, 1) / 3
		case 16:
			prevLagAdj = fix.RShift32(prevLagAdj, 1)
		case 24:
			prevLagAdj = prevLagAdj / 3
		}
		prevLagLog2Q7 = dsp.Lin2Log(prevLagAdj)
	}
	corrThresQ15 := fix.RShift32(fix.SmulBB(searchThres2Q15, searchThres2Q15), 13)

	nbCbksStage2 := int32(silk.PitchEstNBCbksStage2)
	if FsKHz == 8 && complexity > silk.PitchEstMinComplex {
		nbCbksStage2 = silk.PitchEstNBCbksStage2Ext
	}

	var CC [silk.PitchEstNBCbksStage2Ext]int32
	for k := int32(0); k < lengthDSrch; k++ {
		d := dSrch[k]
		for j := int32(0); j < nbCbksStage2; j++ {
			CC[j] = 0
			for i := int32(0); i < silk.PitchEstNBSubFr; i++ {
				CC[j] += int32(C[i][d+int32(tables.CB_lags_stage2[i][j])])
			}
		}
		CCmaxNew := int32(-1 << 31)
		CBimaxNew := int32(0)
		for i := int32(0); i < nbCbksStage2; i++ {
			if CC[i] > CCmaxNew {
				CCmaxNew = CC[i]
				CBimaxNew = i
			}
		}

		lagLog2Q7 := dsp.Lin2Log(d)
		var CCmaxNewB int32
		if forLJC != 0 {
			CCmaxNewB = CCmaxNew
		} else {
			CCmaxNewB = CCmaxNew - fix.RShift32(
				fix.SmulBB(silk.PitchEstNBSubFr*silk.PitchEstShortLagBiasQ15, lagLog2Q7), 7)
		}
		if prevLagAdj > 0 {
			deltaLagLog2SqrQ7 := lagLog2Q7 - prevLagLog2Q7
			deltaLagLog2SqrQ7 = fix.RShift32(fix.SmulBB(deltaLagLog2SqrQ7, deltaLagLog2SqrQ7), 7)
			prevLagBiasQ15 := fix.RShift32(
				fix.SmulBB(silk.PitchEstNBSubFr*silk.PitchEstPrevLagBiasQ15, *LTPCorrQ15), 15)
			prevLagBiasQ15 = (prevLagBiasQ15 * deltaLagLog2SqrQ7) /
				(deltaLagLog2SqrQ7 + (1 << 6))
			CCmaxNewB -= prevLagBiasQ15
		}

		if CCmaxNewB > CCmaxB &&
			CCmaxNew > corrThresQ15 &&
			int32(tables.CB_lags_stage2[0][CBimaxNew]) <= minLag8kHz {
			CCmaxB = CCmaxNewB
			CCmax = CCmaxNew
			lag = d
			CBimax = CBimaxNew
		}
	}

	if lag == -1 {
		for i := range pitchOut[:silk.PitchEstNBSubFr] {
			pitchOut[i] = 0
		}
		*LTPCorrQ15 = 0
		*lagIndex = 0
		*contourIndex = 0
		return 1
	}

	if FsKHz > 8 {
		// Stage 3: refine at the input rate.
		shift = findScaling(signal, frameLength, sfLength)
		var inputSignalPtr []int16
		if shift > 0 {
			// Reuse scratch as int16. Punning via a sized slice — fine in
			// Go because we own the storage.
			inputBuf := unsafeInt32SliceAsInt16(scratchMem[:])
			for i := int32(0); i < frameLength; i++ {
				inputBuf[i] = int16(fix.RShift32(int32(signal[i]), shift))
			}
			inputSignalPtr = inputBuf
		} else {
			inputSignalPtr = signal
		}

		CBimaxOld := CBimax
		switch FsKHz {
		case 12:
			lag = fix.RShift32(fix.SmulBB(lag, 3), 1)
		case 16:
			lag = fix.LShift32(lag, 1)
		default:
			lag = fix.SmulBB(lag, 3)
		}
		lag = fix.Limit(lag, minLag, maxLag)
		startLag := fix.MaxInt(lag-2, minLag)
		endLag := fix.MinInt(lag+2, maxLag)
		lagNew := lag
		CBimax = 0
		*LTPCorrQ15 = fix.SqrtApprox(fix.LShift32(CCmax, 13))

		CCmax = int32(-1 << 31)
		for k := int32(0); k < silk.PitchEstNBSubFr; k++ {
			pitchOut[k] = lag + 2*int32(tables.CB_lags_stage2[k][CBimaxOld])
		}

		var crossCorrSt3 [silk.PitchEstNBSubFr][silk.PitchEstNBCbksStage3Max][silk.PitchEstNBStage3Lags]int32
		var energiesSt3 [silk.PitchEstNBSubFr][silk.PitchEstNBCbksStage3Max][silk.PitchEstNBStage3Lags]int32
		calcCorrSt3(&crossCorrSt3, inputSignalPtr, startLag, sfLength, complexity)
		calcEnergySt3(&energiesSt3, inputSignalPtr, startLag, sfLength, complexity)

		lagCounter := int32(0)
		contourBias := silk.PitchEstFlatContourBiasQ20 / lag

		cbkSize := int32(tables.Cbk_sizes_stage3[complexity])
		cbkOffset := int32(tables.Cbk_offsets_stage3[complexity])

		for d := startLag; d <= endLag; d++ {
			for j := cbkOffset; j < cbkOffset+cbkSize; j++ {
				crossCorr := int32(0)
				e := int32(0)
				for k := int32(0); k < silk.PitchEstNBSubFr; k++ {
					e += fix.RShift32(energiesSt3[k][j][lagCounter], 2)
					crossCorr += fix.RShift32(crossCorrSt3[k][j][lagCounter], 2)
				}
				var CCmaxNew int32
				if crossCorr > 0 {
					lz := fix.Clz32(crossCorr)
					lshift := fix.Limit(lz-1, 0, 13)
					CCmaxNew = fix.LShift32(crossCorr, lshift) / (fix.RShift32(e, 13-lshift) + 1)
					CCmaxNew = fix.Sat16(CCmaxNew)
					CCmaxNew = fix.SmulWB(crossCorr, CCmaxNew)
					if CCmaxNew > fix.RShift32(0x7FFFFFFF, 3) {
						CCmaxNew = 0x7FFFFFFF
					} else {
						CCmaxNew = fix.LShift32(CCmaxNew, 3)
					}
					// Flatness penalty.
					diff := j - fix.RShift32(silk.PitchEstNBCbksStage3Max, 1)
					diff = diff * diff
					diff = 32767 - fix.RShift32(contourBias*diff, 5)
					CCmaxNew = fix.LShift32(fix.SmulWB(CCmaxNew, diff), 1)
				}

				if CCmaxNew > CCmax &&
					(d+int32(tables.CB_lags_stage3[0][j])) <= maxLag {
					CCmax = CCmaxNew
					lagNew = d
					CBimax = j
				}
			}
			lagCounter++
		}

		for k := int32(0); k < silk.PitchEstNBSubFr; k++ {
			pitchOut[k] = lagNew + int32(tables.CB_lags_stage3[k][CBimax])
		}
		*lagIndex = lagNew - minLag
		*contourIndex = CBimax
	} else {
		CCmax = fix.MaxInt(CCmax, 0)
		*LTPCorrQ15 = fix.SqrtApprox(fix.LShift32(CCmax, 13))
		for k := int32(0); k < silk.PitchEstNBSubFr; k++ {
			pitchOut[k] = lag + int32(tables.CB_lags_stage2[k][CBimax])
		}
		*lagIndex = lag - minLag8kHz
		*contourIndex = CBimax
	}
	return 0
}
