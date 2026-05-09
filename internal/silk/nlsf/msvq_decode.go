package nlsf

// MSVQDecode — SKP_Silk_NLSF_MSVQ_decode.
//
// Reconstruct a Q15 NLSF vector from MSVQ stage indices: start with stage 0's
// codebook vector, then add the codebook vector for each subsequent stage.
// Finally call Stabilize to ensure the output satisfies NDeltaMinQ15.
//
// Translated from vendor/silk/src/SKP_Silk_NLSF_MSVQ_decode.c.
func MSVQDecode(nlsfQ15 []int32, cb *CBStruct, indices []int32, lpcOrder int32) {
	// Stage 0: copy the codebook row into the output.
	stage := &cb.CBStages[0]
	off := indices[0] * lpcOrder
	for i := int32(0); i < lpcOrder; i++ {
		nlsfQ15[i] = int32(stage.CBNLSFQ15[off+i])
	}

	for s := int32(1); s < cb.NStages; s++ {
		stage = &cb.CBStages[s]
		off = indices[s] * lpcOrder
		// The C source has a hand-unrolled lpc_order==16 path. Go's compiler
		// inlines and unrolls the small loop fine; keep it generic.
		for i := int32(0); i < lpcOrder; i++ {
			nlsfQ15[i] += int32(stage.CBNLSFQ15[off+i])
		}
	}

	Stabilize(nlsfQ15, cb.NDeltaMinQ15, lpcOrder)
}
