package nlsf

import "github.com/rolandhe/silk-go/internal/silk/fix"

// VQSumError — SKP_Silk_NLSF_VQ_sum_error_FIX.
//
// Compute weighted squared quantization errors between N input vectors and
// K codebook rows of an MSVQ stage. Output is err[N*K] in Q20: row-major,
// err[n*K + i] = sum_m wQ6[n*lpc+m] * (inQ15[n*lpc+m] - cbQ15[i*lpc+m])^2 >> 16.
//
// The C source packs two int16 weights per int32 (Wcpy_Q6) so it can use
// SMLAWB+SMLAWT in tandem on ARM. Go has no such pairing — we use unpacked
// SmlaWB calls; the integer semantics are identical to the LE packed path.
//
// Translated from vendor/silk/src/SKP_Silk_NLSF_VQ_sum_error_FIX.c.
func VQSumError(errQ20 []int32, inQ15 []int32, wQ6 []int32, cbQ15 []int16, N, K, lpcOrder int32) {
	for n := int32(0); n < N; n++ {
		inOff := n * lpcOrder
		errOff := n * K
		for i := int32(0); i < K; i++ {
			var sum int32
			cbOff := i * lpcOrder
			for m := int32(0); m < lpcOrder; m++ {
				diff := inQ15[inOff+m] - int32(cbQ15[cbOff+m])
				sum = fix.SmlaWB(sum, fix.SmulBB(diff, diff), wQ6[m])
			}
			errQ20[errOff+i] = sum
		}
	}
}
