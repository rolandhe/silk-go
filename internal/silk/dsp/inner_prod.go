package dsp

// InnerProdAligned — SKP_Silk_inner_prod_aligned. Returns sum(inVec1[i]*inVec2[i])
// as int32. The C version uses SKP_SMLABB which is a non-overflow-protected
// multiply-accumulate.
//
// Translated from vendor/silk/src/SKP_Silk_inner_prod_aligned.c.
func InnerProdAligned(inVec1, inVec2 []int16, length int32) int32 {
	var sum int32
	for i := int32(0); i < length; i++ {
		sum += int32(inVec1[i]) * int32(inVec2[i])
	}
	return sum
}

// InnerProd16Aligned64 — SKP_Silk_inner_prod16_aligned_64. Same as InnerProdAligned
// but accumulates into int64 to avoid overflow.
//
// Translated from vendor/silk/src/SKP_Silk_inner_prod_aligned.c.
func InnerProd16Aligned64(inVec1, inVec2 []int16, length int32) int64 {
	var sum int64
	for i := int32(0); i < length; i++ {
		sum += int64(int32(inVec1[i]) * int32(inVec2[i]))
	}
	return sum
}
