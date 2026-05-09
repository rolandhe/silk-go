package dsp

// SumSqrShift — SKP_Silk_sum_sqr_shift.
// Compute the sum of squares of x and the right-shift count required to fit
// the result in int32 with at least 2 leading zeros. The two-at-a-time loop
// mirrors the C implementation's accumulation-and-rescale strategy so that
// the shift count is reproducible.
//
// In Go we don't have the C's "is x[] 4-byte aligned?" branch — Go slices
// don't expose alignment and SILK's callers always pass aligned buffers in
// practice — so we always start at i=0.
//
// Translated from vendor/silk/src/SKP_Silk_sum_sqr_shift.c.
func SumSqrShift(x []int16, length int32) (energy int32, shift int32) {
	var nrg int32
	var nrgTmp int32
	var i int32

	end := length - 1
	for i < end {
		// Two values at once: x[i]*x[i] + x[i+1]*x[i+1] using int32 wrap.
		a := int32(x[i])
		b := int32(x[i+1])
		nrg = int32(uint32(nrg) + uint32(a*a))
		nrg = int32(uint32(nrg) + uint32(b*b))
		i += 2
		if nrg < 0 {
			nrg = int32(uint32(nrg) >> 2)
			shift = 2
			break
		}
	}
	for ; i < end; i += 2 {
		a := int32(x[i])
		b := int32(x[i+1])
		nrgTmp = a * a
		nrgTmp = int32(uint32(nrgTmp) + uint32(b*b))
		nrg = int32(uint32(nrg) + uint32(nrgTmp)>>uint(shift))
		if nrg < 0 {
			nrg = int32(uint32(nrg) >> 2)
			shift += 2
		}
	}
	if i == end {
		// One sample left.
		a := int32(x[i])
		nrgTmp = a * a
		nrg = int32(uint32(nrg) + uint32(nrgTmp)>>uint(shift))
	}
	// Ensure at least two leading zeros.
	if uint32(nrg)&0xC0000000 != 0 {
		nrg = int32(uint32(nrg) >> 2)
		shift += 2
	}
	return nrg, shift
}
