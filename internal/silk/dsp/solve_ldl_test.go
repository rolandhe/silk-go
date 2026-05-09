package dsp

import (
	"math"
	"math/rand"
	"testing"
)

// floatLDL — float64 reference: solve A·x = b for symmetric A using LDL.
func floatLDL(A [][]float64, b []float64) []float64 {
	M := len(A)
	L := make([][]float64, M)
	for i := range L {
		L[i] = make([]float64, M)
		L[i][i] = 1
	}
	D := make([]float64, M)
	for j := 0; j < M; j++ {
		s := A[j][j]
		for k := 0; k < j; k++ {
			s -= L[j][k] * L[j][k] * D[k]
		}
		D[j] = s
		for i := j + 1; i < M; i++ {
			t := A[i][j]
			for k := 0; k < j; k++ {
				t -= L[i][k] * L[j][k] * D[k]
			}
			L[i][j] = t / D[j]
		}
	}
	// Solve L·y = b
	y := make([]float64, M)
	for i := 0; i < M; i++ {
		t := b[i]
		for j := 0; j < i; j++ {
			t -= L[i][j] * y[j]
		}
		y[i] = t
	}
	// y = D⁻¹·y
	for i := 0; i < M; i++ {
		y[i] /= D[i]
	}
	// Solve L'·x = y
	x := make([]float64, M)
	for i := M - 1; i >= 0; i-- {
		t := y[i]
		for j := i + 1; j < M; j++ {
			t -= L[j][i] * x[j]
		}
		x[i] = t
	}
	return x
}

// makeSPDMatrix — generate a strongly diagonally-dominant SPD matrix +
// matching b vector. SILK's LDL is approximate (uses int32 inversions); on
// poorly-conditioned A the answer drifts. Diagonal dominance keeps it sane.
func makeSPDMatrix(rng *rand.Rand, M int32) ([]int32, []int32, [][]float64, []float64) {
	aF := make([][]float64, M)
	for i := range aF {
		aF[i] = make([]float64, M)
	}
	// Random off-diagonals in [-1, 1].
	for i := int32(0); i < M; i++ {
		for j := i + 1; j < M; j++ {
			v := float64(rng.Intn(201)-100) / 100.0
			aF[i][j] = v
			aF[j][i] = v
		}
	}
	// Diagonals: row sum + extra to ensure strong dominance.
	for i := int32(0); i < M; i++ {
		s := 0.0
		for j := int32(0); j < M; j++ {
			if i != j {
				s += math.Abs(aF[i][j])
			}
		}
		aF[i][i] = s + float64(rng.Intn(10)+5)
	}

	bF := make([]float64, M)
	for i := range bF {
		bF[i] = float64(rng.Intn(2001) - 1000)
	}

	// Scale A and b into integer ranges. We pick the same scale so that
	// both fit comfortably in int32. SILK's LDL_factorize is sized for
	// autocorrelation matrices where the diagonal sits in ~2^29; below ~2^17
	// the two-step inverse (oneDivQ36 << 4) overflows. Cap at the smaller
	// of: (a) what keeps A near 2^25, or (b) what keeps b within int32.
	scale := 1.0
	maxA := 0.0
	for i := range aF {
		for _, v := range aF[i] {
			if math.Abs(v) > maxA {
				maxA = math.Abs(v)
			}
		}
	}
	maxB := 0.0
	for _, v := range bF {
		if math.Abs(v) > maxB {
			maxB = math.Abs(v)
		}
	}
	if maxA > 0 {
		scaleA := float64(int64(1)<<25) / maxA
		scaleB := float64(int64(1)<<29) / (maxB + 1)
		scale = math.Min(scaleA, scaleB)
	}

	A := make([]int32, M*M)
	for i := int32(0); i < M; i++ {
		for j := int32(0); j < M; j++ {
			A[i*M+j] = int32(aF[i][j] * scale)
		}
	}
	b := make([]int32, M)
	for i := int32(0); i < M; i++ {
		b[i] = int32(bF[i] * scale)
	}
	return A, b, aF, bF
}

// TestSolveLDLAgainstFloat — solve a few random SPD systems and compare
// the Q16 integer solution against a float reference.
func TestSolveLDLAgainstFloat(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 20; trial++ {
		M := int32(2 + (trial % 6)) // 2..7
		A, b, aF, bF := makeSPDMatrix(rng, M)
		// Reference solution.
		xRef := floatLDL(aF, bF)

		// Save A copy: SolveLDL may inflate the diagonal on regularisation.
		Acopy := append([]int32(nil), A...)
		xQ16 := make([]int32, M)
		SolveLDL(Acopy, M, b, xQ16)

		// Sanity: verify the float reference by checking A·xRef ≈ b.
		for i := int32(0); i < M; i++ {
			var residual float64
			for j := int32(0); j < M; j++ {
				residual += aF[i][j] * xRef[j]
			}
			if math.Abs(residual-bF[i]) > math.Abs(bF[i])*0.001+1e-3 {
				t.Logf("trial %d: float reference: row %d residual=%v want %v aF=%v bF=%v xRef=%v",
					trial, i, residual, bF[i], aF, bF, xRef)
				t.Fatalf("trial %d: float reference broken", trial)
			}
		}

		for i := int32(0); i < M; i++ {
			gotF := float64(xQ16[i]) / 65536.0
			diff := math.Abs(gotF - xRef[i])
			rel := diff / (math.Abs(xRef[i]) + 1e-9)
			if diff > 0.05 && rel > 0.02 {
				t.Errorf("trial %d M=%d idx %d: got %.4f ref %.4f", trial, M, i, gotF, xRef[i])
			}
		}
	}
}

// TestSolveLDLIdentity — A = c·I should give x = b/c.
func TestSolveLDLIdentity(t *testing.T) {
	M := int32(4)
	A := make([]int32, M*M)
	for i := int32(0); i < M; i++ {
		A[i*M+i] = 1 << 20 // c = 2^20
	}
	b := []int32{1 << 20, 2 << 20, 3 << 20, 4 << 20}
	xQ16 := make([]int32, M)
	SolveLDL(append([]int32(nil), A...), M, b, xQ16)

	// Expected x = [1, 2, 3, 4] in real, = [1<<16, 2<<16, ...] in Q16.
	for i := int32(0); i < M; i++ {
		want := int32(i+1) << 16
		diff := xQ16[i] - want
		if diff < -64 || diff > 64 {
			t.Errorf("idx %d: got %d want %d", i, xQ16[i], want)
		}
	}
}

// TestRegularizeCorrelations — adds noise to diagonal + xx[0].
func TestRegularizeCorrelations(t *testing.T) {
	D := int32(4)
	XX := []int32{
		10, 1, 2, 3,
		1, 20, 4, 5,
		2, 4, 30, 6,
		3, 5, 6, 40,
	}
	xx := []int32{100, 200, 300, 400}
	noise := int32(7)
	RegularizeCorrelations(XX, xx, noise, D)
	wantDiag := []int32{17, 27, 37, 47}
	for i := int32(0); i < D; i++ {
		if XX[i*D+i] != wantDiag[i] {
			t.Errorf("XX[%d,%d]=%d want %d", i, i, XX[i*D+i], wantDiag[i])
		}
	}
	if xx[0] != 107 {
		t.Errorf("xx[0]=%d want 107", xx[0])
	}
	// Off-diagonals untouched.
	if XX[0*D+1] != 1 || XX[2*D+0] != 2 {
		t.Errorf("off-diagonals modified: XX[0,1]=%d XX[2,0]=%d", XX[0*D+1], XX[2*D+0])
	}
}

// TestCorrMatrixSymmetric — the output XX must be symmetric.
func TestCorrMatrixSymmetric(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for trial := 0; trial < 10; trial++ {
		L := int32(64 + rng.Intn(64))
		order := int32(4 + (rng.Intn(5)))
		x := make([]int16, L+order-1)
		for i := range x {
			x[i] = int16(rng.Intn(2001) - 1000)
		}
		XX := make([]int32, order*order)
		var rshifts int32
		CorrMatrix(x, L, order, 1, XX, &rshifts)
		for i := int32(0); i < order; i++ {
			for j := i + 1; j < order; j++ {
				if XX[i*order+j] != XX[j*order+i] {
					t.Errorf("trial %d (%d,%d): %d vs %d (asymmetric)",
						trial, i, j, XX[i*order+j], XX[j*order+i])
				}
			}
		}
		// Diagonal should be non-negative (it's a Gram matrix entry).
		for i := int32(0); i < order; i++ {
			if XX[i*order+i] < 0 {
				t.Errorf("trial %d: XX[%d,%d]=%d < 0", trial, i, i, XX[i*order+i])
			}
		}
	}
}

// TestCorrVectorAgainstFloat — float reference for the X'·t correlation.
func TestCorrVectorAgainstFloat(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	L := int32(64)
	order := int32(8)
	x := make([]int16, L+order-1)
	tt := make([]int16, L)
	for i := range x {
		x[i] = int16(rng.Intn(2001) - 1000)
	}
	for i := range tt {
		tt[i] = int16(rng.Intn(2001) - 1000)
	}
	Xt := make([]int32, order)
	CorrVector(x, tt, L, order, Xt, 0)
	for lag := int32(0); lag < order; lag++ {
		var ref int32
		for i := int32(0); i < L; i++ {
			ref += int32(x[order-1-lag+i]) * int32(tt[i])
		}
		if Xt[lag] != ref {
			t.Errorf("lag %d: got %d want %d", lag, Xt[lag], ref)
		}
	}
}
