package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// ScaleCopyVector16 — SKP_Silk_scale_copy_vector16.
// Copy data_in to data_out scaled by gain (Q16).
//
// Translated from vendor/silk/src/SKP_Silk_scale_copy_vector16.c.
func ScaleCopyVector16(out []int16, in []int16, gainQ16 int32, dataSize int32) {
	for i := int32(0); i < dataSize; i++ {
		out[i] = int16(fix.SmulWB(gainQ16, int32(in[i])))
	}
}

// ScaleVector32Q26Lshift18 — SKP_Silk_scale_vector32_Q26_lshift_18.
// In-place: data1[i] = (int64(data1[i]) * gainQ26) >> 8 (output Q18 from Q0
// input or Q44 → Q18 from Q26 multiplied by Q0). The C version asserts that
// the int64-shifted result fits in int32; we trust that constraint and let
// any overflow truncate to match.
//
// Translated from vendor/silk/src/SKP_Silk_scale_vector.c.
func ScaleVector32Q26Lshift18(data []int32, gainQ26 int32, dataSize int32) {
	for i := int32(0); i < dataSize; i++ {
		data[i] = int32(fix.RShift64(fix.Smull(data[i], gainQ26), 8))
	}
}
