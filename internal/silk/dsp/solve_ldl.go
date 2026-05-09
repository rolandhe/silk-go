package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// MaxMatrixSize mirrors MAX_MATRIX_SIZE from SKP_Silk_define.h (= MAX_LPC_ORDER = 16).
const MaxMatrixSize = 16

// invD is the per-diagonal-element inverse stored as two parts (Q36 + Q48)
// to support the two-step division in LDL_factorize. Mirrors the inv_D_t
// struct from SKP_Silk_solve_LS_FIX.c.
type invD struct {
	Q36Part int32
	Q48Part int32
}

// SolveLDL — SKP_Silk_solve_LDL_FIX.
//
// Solve A·x = b for symmetric A by LDL factorisation. Output xQ16 in Q16.
// A is mutated in place by the factorisation when the matrix needs
// regularisation; on return its diagonal may be inflated.
//
// Translated from vendor/silk/src/SKP_Silk_solve_LS_FIX.c.
func SolveLDL(A []int32, M int32, b []int32, xQ16 []int32) {
	var LQ16 [MaxMatrixSize * MaxMatrixSize]int32
	var Y [MaxMatrixSize]int32
	var inv [MaxMatrixSize]invD

	ldlFactorize(A, M, LQ16[:], inv[:])
	lsSolveFirst(LQ16[:], M, b, Y[:])
	lsDivideQ16(Y[:], inv[:], M)
	lsSolveLast(LQ16[:], M, Y[:], xQ16)
}

// ldlFactorize — SKP_Silk_LDL_factorize_FIX.
//
// Factor A = L·D·L' with L unit-lower-triangular. inv[j] holds 1/D[j].
// If the matrix is not positive semi-definite, the diagonal is inflated
// and the factorisation retried (up to M times).
func ldlFactorize(A []int32, M int32, LQ16 []int32, inv []invD) {
	// SKP_FIX_CONST(FIND_LTP_COND_FAC, 31) = round(1e-5 * 2^31) = 21475.
	const findLTPCondFacQ31 = int32(21475)

	matrixGet := func(base []int32, row, col int32) int32 { return base[row*M+col] }
	matrixSet := func(base []int32, row, col, v int32) { base[row*M+col] = v }

	var vQ0 [MaxMatrixSize]int32
	var DQ0 [MaxMatrixSize]int32

	diagMin := fix.Smmul(fix.AddSat32(A[0], A[M*M-1]), findLTPCondFacQ31)
	if diagMin < 1<<9 {
		diagMin = 1 << 9
	}

	status := int32(1)
	for loop := int32(0); loop < M && status == 1; loop++ {
		status = 0
		for j := int32(0); j < M; j++ {
			// Row j of L_Q16 (under-diagonal entries built so far).
			rowOff := j * M
			var tmp32 int32
			for i := int32(0); i < j; i++ {
				vQ0[i] = fix.SmulWW(DQ0[i], LQ16[rowOff+i])
				tmp32 = fix.SmlaWW(tmp32, vQ0[i], LQ16[rowOff+i])
			}
			tmp32 = matrixGet(A, j, j) - tmp32

			if tmp32 < diagMin {
				bump := fix.SmulBB(loop+1, diagMin) - tmp32
				for i := int32(0); i < M; i++ {
					matrixSet(A, i, i, matrixGet(A, i, i)+bump)
				}
				status = 1
				break
			}
			DQ0[j] = tmp32

			// Two-step division: 1/tmp32 in Q36 and refined to Q48.
			oneDivQ36 := fix.Inverse32VarQ(tmp32, 36)
			oneDivQ40 := fix.LShift32(oneDivQ36, 4)
			err := (int32(1) << 24) - fix.SmulWW(tmp32, oneDivQ40)
			oneDivQ48 := fix.SmulWW(err, oneDivQ40)
			inv[j].Q36Part = oneDivQ36
			inv[j].Q48Part = oneDivQ48

			matrixSet(LQ16, j, j, 65536) // 1.0 in Q16

			// Below-diagonal entries of column j.
			ARowJ := j * M
			for i := j + 1; i < M; i++ {
				LRowI := i * M
				var t int32
				for k := int32(0); k < j; k++ {
					t = fix.SmlaWW(t, vQ0[k], LQ16[LRowI+k])
				}
				t = A[ARowJ+i] - t
				LQ16[LRowI+j] = fix.Smmul(t, oneDivQ48) +
					fix.RShift32(fix.SmulWW(t, oneDivQ36), 4)
			}
		}
	}
}

// lsDivideQ16 — SKP_Silk_LS_divide_Q16_FIX. T[i] /= D[i].
func lsDivideQ16(T []int32, inv []invD, M int32) {
	for i := int32(0); i < M; i++ {
		t := T[i]
		T[i] = fix.Smmul(t, inv[i].Q48Part) +
			fix.RShift32(fix.SmulWW(t, inv[i].Q36Part), 4)
	}
}

// lsSolveFirst — SKP_Silk_LS_SolveFirst_FIX.
// Solve L·x = b with unit-diagonal L (forward substitution).
func lsSolveFirst(LQ16 []int32, M int32, b []int32, xQ16 []int32) {
	for i := int32(0); i < M; i++ {
		row := i * M
		var t int32
		for j := int32(0); j < i; j++ {
			t = fix.SmlaWW(t, LQ16[row+j], xQ16[j])
		}
		xQ16[i] = b[i] - t
	}
}

// lsSolveLast — SKP_Silk_LS_SolveLast_FIX.
// Solve L'·x = b (back substitution). The C source uses column access on
// the same row-major storage by computing matrix_adr(L, 0, i, M)+j*M, i.e.
// LQ16[i + j*M].
func lsSolveLast(LQ16 []int32, M int32, b []int32, xQ16 []int32) {
	for i := M - 1; i >= 0; i-- {
		var t int32
		for j := M - 1; j > i; j-- {
			t = fix.SmlaWW(t, LQ16[j*M+i], xQ16[j])
		}
		xQ16[i] = b[i] - t
	}
}
