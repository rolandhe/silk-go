package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Autocorr — SKP_Silk_autocorr.
//
// Compute the autocorrelation of `inputData`, output `correlationCount` taps
// scaled to fit in int32. `*scale` receives the right-shift count applied
// (negative means the result was left-shifted instead). Lag 0 is forced to
// have at least 29 non-zero bits to leave headroom for white-noise add-ons.
//
// Translated from vendor/silk/src/SKP_Silk_autocorr.c.
func Autocorr(results []int32, scale *int32, inputData []int16, inputDataSize int32, correlationCount int32) {
	corrCount := inputDataSize
	if correlationCount < corrCount {
		corrCount = correlationCount
	}

	// Lag-0 (energy) in int64 to avoid overflow.
	corr64 := InnerProd16Aligned64(inputData, inputData, inputDataSize)
	corr64 += 1 // protect against an all-zero input

	lz := fix.Clz64(corr64)
	nRightShifts := 35 - lz
	*scale = nRightShifts

	if nRightShifts <= 0 {
		results[0] = int32(corr64) << uint(-nRightShifts)
		for i := int32(1); i < corrCount; i++ {
			results[i] = InnerProdAligned(inputData, inputData[i:], inputDataSize-i) << uint(-nRightShifts)
		}
	} else {
		results[0] = int32(corr64 >> uint(nRightShifts))
		for i := int32(1); i < corrCount; i++ {
			results[i] = int32(InnerProd16Aligned64(inputData, inputData[i:], inputDataSize-i) >> uint(nRightShifts))
		}
	}
}
