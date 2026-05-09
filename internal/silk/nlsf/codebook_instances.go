package nlsf

import "github.com/rolandhe/silk-go/internal/silk/tables"

// Hand-written codebook instances corresponding to the four
// SKP_Silk_NLSF_CB[01]_(10|16) globals in
// vendor/silk/src/SKP_Silk_tables_NLSF_CB*_*.c. The stage_info / CDF
// pointer arrays can't be machine-translated by tablegen, so we build
// them here from the underlying flat tables.

func slice2D(flat []int16, rowLen int, off int) []int16 {
	return flat[off*rowLen:]
}

// CB0_10 — voiced 10-th order codebook.
var CB0_10 = CBStruct{
	NStages: 6,
	CBStages: []CBStage{
		{NVectors: 64, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_10_Q15[:], 10, 0), RatesQ5: tables.NLSF_MSVQ_CB0_10_rates_Q5[0:]},
		{NVectors: 16, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_10_Q15[:], 10, 64), RatesQ5: tables.NLSF_MSVQ_CB0_10_rates_Q5[64:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_10_Q15[:], 10, 80), RatesQ5: tables.NLSF_MSVQ_CB0_10_rates_Q5[80:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_10_Q15[:], 10, 88), RatesQ5: tables.NLSF_MSVQ_CB0_10_rates_Q5[88:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_10_Q15[:], 10, 96), RatesQ5: tables.NLSF_MSVQ_CB0_10_rates_Q5[96:]},
		{NVectors: 16, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_10_Q15[:], 10, 104), RatesQ5: tables.NLSF_MSVQ_CB0_10_rates_Q5[104:]},
	},
	NDeltaMinQ15: tables.NLSF_MSVQ_CB0_10_ndelta_min_Q15[:],
	CDF:          tables.NLSF_MSVQ_CB0_10_CDF[:],
	StartPtr: [][]uint16{
		tables.NLSF_MSVQ_CB0_10_CDF[0:],
		tables.NLSF_MSVQ_CB0_10_CDF[65:],
		tables.NLSF_MSVQ_CB0_10_CDF[82:],
		tables.NLSF_MSVQ_CB0_10_CDF[91:],
		tables.NLSF_MSVQ_CB0_10_CDF[100:],
		tables.NLSF_MSVQ_CB0_10_CDF[109:],
	},
	MiddleIx: tables.NLSF_MSVQ_CB0_10_CDF_middle_idx[:],
}

// CB0_16 — voiced 16-th order codebook.
var CB0_16 = CBStruct{
	NStages: 10,
	CBStages: []CBStage{
		{NVectors: 128, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 0), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[0:]},
		{NVectors: 16, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 128), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[128:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 144), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[144:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 152), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[152:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 160), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[160:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 168), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[168:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 176), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[176:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 184), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[184:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 192), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[192:]},
		{NVectors: 16, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB0_16_Q15[:], 16, 200), RatesQ5: tables.NLSF_MSVQ_CB0_16_rates_Q5[200:]},
	},
	NDeltaMinQ15: tables.NLSF_MSVQ_CB0_16_ndelta_min_Q15[:],
	CDF:          tables.NLSF_MSVQ_CB0_16_CDF[:],
	StartPtr: [][]uint16{
		tables.NLSF_MSVQ_CB0_16_CDF[0:],
		tables.NLSF_MSVQ_CB0_16_CDF[129:],
		tables.NLSF_MSVQ_CB0_16_CDF[146:],
		tables.NLSF_MSVQ_CB0_16_CDF[155:],
		tables.NLSF_MSVQ_CB0_16_CDF[164:],
		tables.NLSF_MSVQ_CB0_16_CDF[173:],
		tables.NLSF_MSVQ_CB0_16_CDF[182:],
		tables.NLSF_MSVQ_CB0_16_CDF[191:],
		tables.NLSF_MSVQ_CB0_16_CDF[200:],
		tables.NLSF_MSVQ_CB0_16_CDF[209:],
	},
	MiddleIx: tables.NLSF_MSVQ_CB0_16_CDF_middle_idx[:],
}

// CB1_10 — unvoiced 10-th order codebook.
var CB1_10 = CBStruct{
	NStages: 6,
	CBStages: []CBStage{
		{NVectors: 32, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_10_Q15[:], 10, 0), RatesQ5: tables.NLSF_MSVQ_CB1_10_rates_Q5[0:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_10_Q15[:], 10, 32), RatesQ5: tables.NLSF_MSVQ_CB1_10_rates_Q5[32:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_10_Q15[:], 10, 40), RatesQ5: tables.NLSF_MSVQ_CB1_10_rates_Q5[40:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_10_Q15[:], 10, 48), RatesQ5: tables.NLSF_MSVQ_CB1_10_rates_Q5[48:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_10_Q15[:], 10, 56), RatesQ5: tables.NLSF_MSVQ_CB1_10_rates_Q5[56:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_10_Q15[:], 10, 64), RatesQ5: tables.NLSF_MSVQ_CB1_10_rates_Q5[64:]},
	},
	NDeltaMinQ15: tables.NLSF_MSVQ_CB1_10_ndelta_min_Q15[:],
	CDF:          tables.NLSF_MSVQ_CB1_10_CDF[:],
	StartPtr: [][]uint16{
		tables.NLSF_MSVQ_CB1_10_CDF[0:],
		tables.NLSF_MSVQ_CB1_10_CDF[33:],
		tables.NLSF_MSVQ_CB1_10_CDF[42:],
		tables.NLSF_MSVQ_CB1_10_CDF[51:],
		tables.NLSF_MSVQ_CB1_10_CDF[60:],
		tables.NLSF_MSVQ_CB1_10_CDF[69:],
	},
	MiddleIx: tables.NLSF_MSVQ_CB1_10_CDF_middle_idx[:],
}

// CB1_16 — unvoiced 16-th order codebook.
var CB1_16 = CBStruct{
	NStages: 10,
	CBStages: []CBStage{
		{NVectors: 32, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 0), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[0:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 32), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[32:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 40), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[40:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 48), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[48:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 56), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[56:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 64), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[64:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 72), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[72:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 80), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[80:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 88), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[88:]},
		{NVectors: 8, CBNLSFQ15: slice2D(tables.NLSF_MSVQ_CB1_16_Q15[:], 16, 96), RatesQ5: tables.NLSF_MSVQ_CB1_16_rates_Q5[96:]},
	},
	NDeltaMinQ15: tables.NLSF_MSVQ_CB1_16_ndelta_min_Q15[:],
	CDF:          tables.NLSF_MSVQ_CB1_16_CDF[:],
	StartPtr: [][]uint16{
		tables.NLSF_MSVQ_CB1_16_CDF[0:],
		tables.NLSF_MSVQ_CB1_16_CDF[33:],
		tables.NLSF_MSVQ_CB1_16_CDF[42:],
		tables.NLSF_MSVQ_CB1_16_CDF[51:],
		tables.NLSF_MSVQ_CB1_16_CDF[60:],
		tables.NLSF_MSVQ_CB1_16_CDF[69:],
		tables.NLSF_MSVQ_CB1_16_CDF[78:],
		tables.NLSF_MSVQ_CB1_16_CDF[87:],
		tables.NLSF_MSVQ_CB1_16_CDF[96:],
		tables.NLSF_MSVQ_CB1_16_CDF[105:],
	},
	MiddleIx: tables.NLSF_MSVQ_CB1_16_CDF_middle_idx[:],
}
