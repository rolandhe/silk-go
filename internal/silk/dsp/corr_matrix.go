package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// CorrVector — SKP_Silk_corrVector_FIX.
//
// Compute X'·t where X is the [L × order] data matrix derived from `x`
// (column j of X is x[order-1-j … order-1-j+L-1]). Output Xt has length
// `order`. `rshifts` controls right-shift applied per multiplication.
//
// Translated from vendor/silk/src/SKP_Silk_corrMatrix_FIX.c.
func CorrVector(x []int16, t []int16, L int32, order int32, Xt []int32, rshifts int32) {
	if rshifts > 0 {
		for lag := int32(0); lag < order; lag++ {
			ptr1 := x[order-1-lag:]
			var inner int32
			for i := int32(0); i < L; i++ {
				inner += fix.RShift32(fix.SmulBB(int32(ptr1[i]), int32(t[i])), rshifts)
			}
			Xt[lag] = inner
		}
	} else {
		// rshifts must be 0.
		for lag := int32(0); lag < order; lag++ {
			ptr1 := x[order-1-lag:]
			Xt[lag] = InnerProdAligned(ptr1, t, L)
		}
	}
}

// CorrMatrix — SKP_Silk_corrMatrix_FIX.
//
// Compute the [order × order] correlation matrix XX = X'·X (row-major,
// length order*order). `rshifts` is in/out: caller passes a desired minimum,
// the function may bump it up based on signal energy and head-room budget.
//
// Translated from vendor/silk/src/SKP_Silk_corrMatrix_FIX.c.
func CorrMatrix(x []int16, L int32, order int32, headRoom int32, XX []int32, rshifts *int32) {
	matrixSet := func(row, col int32, v int32) { XX[row*order+col] = v }

	// Energy and shift.
	energy, rshiftsLocal := SumSqrShift(x, L+order-1)
	headRoomShifts := headRoom - fix.Clz32(energy)
	if headRoomShifts < 0 {
		headRoomShifts = 0
	}
	energy = fix.RShift32(energy, headRoomShifts)
	rshiftsLocal += headRoomShifts

	// Energy of column 0: subtract the first (order-1) samples that aren't part of X[:,0].
	for i := int32(0); i < order-1; i++ {
		energy -= fix.RShift32(fix.SmulBB(int32(x[i]), int32(x[i])), rshiftsLocal)
	}
	if rshiftsLocal < *rshifts {
		energy = fix.RShift32(energy, *rshifts-rshiftsLocal)
		rshiftsLocal = *rshifts
	}

	matrixSet(0, 0, energy)
	ptr1 := x[order-1:]
	// Rolling diagonal: X[:,j]'·X[:,j].
	for j := int32(1); j < order; j++ {
		energy = fix.Sub32(energy, fix.RShift32(fix.SmulBB(int32(ptr1[L-j]), int32(ptr1[L-j])), rshiftsLocal))
		// ptr1[-j] in C = x[order-1-j].
		v := int32(x[order-1-j])
		energy = fix.Add32(energy, fix.RShift32(fix.SmulBB(v, v), rshiftsLocal))
		matrixSet(j, j, energy)
	}

	ptr2Off := int32(order - 2) // x index where ptr2[0] starts; ptr2 = &x[order-2]
	if rshiftsLocal > 0 {
		for lag := int32(1); lag < order; lag++ {
			ptr2 := x[ptr2Off:]
			var energy int32
			for i := int32(0); i < L; i++ {
				energy += fix.RShift32(fix.SmulBB(int32(ptr1[i]), int32(ptr2[i])), rshiftsLocal)
			}
			matrixSet(lag, 0, energy)
			matrixSet(0, lag, energy)
			for j := int32(1); j < order-lag; j++ {
				energy = fix.Sub32(energy, fix.RShift32(fix.SmulBB(int32(ptr1[L-j]), int32(ptr2[L-j])), rshiftsLocal))
				// ptr1[-j] = x[order-1-j], ptr2[-j] = x[order-2-lag-j+1] = x[order-1-lag-j].
				p1n := int32(x[order-1-j])
				p2n := int32(x[order-1-lag-j])
				energy = fix.Add32(energy, fix.RShift32(fix.SmulBB(p1n, p2n), rshiftsLocal))
				matrixSet(lag+j, j, energy)
				matrixSet(j, lag+j, energy)
			}
			ptr2Off--
		}
	} else {
		for lag := int32(1); lag < order; lag++ {
			ptr2 := x[ptr2Off:]
			energy := InnerProdAligned(ptr1, ptr2, L)
			matrixSet(lag, 0, energy)
			matrixSet(0, lag, energy)
			for j := int32(1); j < order-lag; j++ {
				energy = fix.Sub32(energy, fix.SmulBB(int32(ptr1[L-j]), int32(ptr2[L-j])))
				p1n := int32(x[order-1-j])
				p2n := int32(x[order-1-lag-j])
				energy = fix.SmlaBB(energy, p1n, p2n)
				matrixSet(lag+j, j, energy)
				matrixSet(j, lag+j, energy)
			}
			ptr2Off--
		}
	}
	*rshifts = rshiftsLocal
}
