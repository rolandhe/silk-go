// Package dsp ports the small DSP/utility functions from
// vendor/silk/src/SKP_Silk_*.c that are used across the encoder/decoder.
package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Int16ArrayMaxAbs — SKP_Silk_int16_array_maxabs.
// Returns the maximum absolute value in vec, capped at int16_max
// (32767) when the squared magnitude would exceed (2^15-1)^2.
//
// Translated from vendor/silk/src/SKP_Silk_array_maxabs.c.
func Int16ArrayMaxAbs(vec []int16, length int32) int16 {
	if length == 0 {
		return 0
	}
	ind := length - 1
	max := fix.SmulBB(int32(vec[ind]), int32(vec[ind]))
	for i := length - 2; i >= 0; i-- {
		lvl := fix.SmulBB(int32(vec[i]), int32(vec[i]))
		if lvl > max {
			max = lvl
			ind = i
		}
	}
	// Do not return 32768, as it will not fit in int16 (2^15-1)^2 = 1073676289.
	if max >= 1073676289 {
		return 32767
	}
	if vec[ind] < 0 {
		return -vec[ind]
	}
	return vec[ind]
}
