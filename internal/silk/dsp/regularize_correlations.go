package dsp

// RegularizeCorrelations — SKP_Silk_regularize_correlations_FIX.
//
// Add `noise` to the diagonal of the row-major correlation matrix XX
// ([D × D]) and to xx[0]. Stabilises an ill-conditioned matrix before
// LDL factorisation.
//
// Translated from vendor/silk/src/SKP_Silk_regularize_correlations_FIX.c.
func RegularizeCorrelations(XX []int32, xx []int32, noise int32, D int32) {
	for i := int32(0); i < D; i++ {
		XX[i*D+i] += noise
	}
	xx[0] += noise
}
