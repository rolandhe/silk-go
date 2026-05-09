package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// ResidualEnergy16Covar — SKP_Silk_residual_energy16_covar_FIX.
//
// Compute   nrg = wxx − 2·wXx·c + c'·wXX·c
// where wXX is symmetric and c is in QcQ. Output Q0.
//
// D ≤ MaxMatrixSize, 0 < cQ < 16.
//
// Translated from vendor/silk/src/SKP_Silk_residual_energy16_FIX.c.
func ResidualEnergy16Covar(c []int16, wXX []int32, wXx []int32, wxx int32, D, cQ int32) int32 {
	lshifts := 16 - cQ
	qXtra := lshifts

	var cMax int32
	for i := int32(0); i < D; i++ {
		v := fix.Abs32(int32(c[i]))
		if v > cMax {
			cMax = v
		}
	}
	if cz := fix.Clz32(cMax) - 17; cz < qXtra {
		qXtra = cz
	}

	wMax := wXX[0]
	if v := wXX[D*D-1]; v > wMax {
		wMax = v
	}
	bound := fix.Clz32(fix.Mul(D, fix.RShift32(fix.SmulWB(wMax, cMax), 4))) - 5
	if bound < qXtra {
		qXtra = bound
	}
	if qXtra < 0 {
		qXtra = 0
	}

	var cn [MaxMatrixSize]int32
	for i := int32(0); i < D; i++ {
		cn[i] = fix.LShift32(int32(c[i]), qXtra)
	}
	lshifts -= qXtra

	// wxx − 2·wXx·c
	var tmp int32
	for i := int32(0); i < D; i++ {
		tmp = fix.SmlaWB(tmp, wXx[i], cn[i])
	}
	nrg := fix.RShift32(wxx, 1+lshifts) - tmp

	// c' · wXX · c (assumes wXX symmetric).
	var tmp2 int32
	for i := int32(0); i < D; i++ {
		var t int32
		row := i * D
		for j := i + 1; j < D; j++ {
			t = fix.SmlaWB(t, wXX[row+j], cn[j])
		}
		t = fix.SmlaWB(t, fix.RShift32(wXX[row+i], 1), cn[i])
		tmp2 = fix.SmlaWB(tmp2, t, cn[i])
	}
	nrg = fix.AddLShift32(nrg, tmp2, lshifts)

	if nrg < 1 {
		return 1
	}
	if nrg > fix.RShift32(0x7FFFFFFF, lshifts+2) {
		return 0x7FFFFFFF >> 1
	}
	return fix.LShift32(nrg, lshifts+1)
}
