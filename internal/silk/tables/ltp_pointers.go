package tables

// Hand-written pointer tables that tablegen can't produce: arrays of arrays
// referencing other tables in this package. Translated from
// vendor/silk/src/SKP_Silk_tables_LTP.c (the `* const X[N] = { ... }` blocks).

// LTPGainBITSQ6Ptrs — slices of code-length tables, indexed by codebook id.
// Mirrors SKP_Silk_LTP_gain_BITS_Q6_ptrs.
var LTPGainBITSQ6Ptrs = [3][]int16{
	LTP_gain_BITS_Q6_0[:],
	LTP_gain_BITS_Q6_1[:],
	LTP_gain_BITS_Q6_2[:],
}

// LTPGainCDFPtrs — slices of LTP gain CDFs, indexed by codebook id.
// Mirrors SKP_Silk_LTP_gain_CDF_ptrs.
var LTPGainCDFPtrs = [3][]uint16{
	LTP_gain_CDF_0[:],
	LTP_gain_CDF_1[:],
	LTP_gain_CDF_2[:],
}

// LTPVQPtrsQ14 — slices of LTP gain codebooks (flattened to 1-D so the
// existing VQWMatECFix / sum_error helpers can index them as [n_vec * 5]).
// Mirrors SKP_Silk_LTP_vq_ptrs_Q14.
var LTPVQPtrsQ14 = [3][]int16{
	flattenLTPVQ0(),
	flattenLTPVQ1(),
	flattenLTPVQ2(),
}

func flattenLTPVQ0() []int16 {
	const N = 10
	out := make([]int16, N*5)
	for i := 0; i < N; i++ {
		copy(out[i*5:], LTP_gain_vq_0_Q14[i][:])
	}
	return out
}

func flattenLTPVQ1() []int16 {
	const N = 20
	out := make([]int16, N*5)
	for i := 0; i < N; i++ {
		copy(out[i*5:], LTP_gain_vq_1_Q14[i][:])
	}
	return out
}

func flattenLTPVQ2() []int16 {
	const N = 40
	out := make([]int16, N*5)
	for i := 0; i < N; i++ {
		copy(out[i*5:], LTP_gain_vq_2_Q14[i][:])
	}
	return out
}
