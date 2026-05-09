package nlsf

// CBStage mirrors SKP_Silk_NLSF_CBS from vendor/silk/src/SKP_Silk_structs.h.
//
// CB_NLSF_Q15 is a flat NVectors × LPC_order int16 buffer (row-major). The
// column dimension is implicit — encoder/decoder pass LPC_order separately.
type CBStage struct {
	NVectors  int32
	CBNLSFQ15 []int16
	RatesQ5   []int16
}

// CBStruct mirrors SKP_Silk_NLSF_CB_struct from vendor/silk/src/SKP_Silk_structs.h.
//
// Holds an MSVQ codebook for one NLSF order (10 or 16). The (de)quant fields
// are CBStages + NDeltaMinQ15; the entropy coder fields are CDF / StartPtr /
// MiddleIx.
type CBStruct struct {
	NStages      int32
	CBStages     []CBStage
	NDeltaMinQ15 []int32
	CDF          []uint16
	StartPtr     [][]uint16
	MiddleIx     []int32
}
