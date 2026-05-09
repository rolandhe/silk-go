package ltp

import (
	"math/rand"
	"testing"
)

// referenceVQWMatEC is a straightforward Go reimplementation that does the
// 5×5 quadratic form using int64 arithmetic on the same SmlaWB-like
// rounding semantics. It serves only as a sanity oracle for selected
// inputs — not as a bit-exact replica.
//
// We don't try to bit-match the production function; instead we test that
// the production function picks the correct codebook entry on hand-built
// inputs with a clear winner.

// TestVQWMatECPicksZeroDistortionMatch — when the input exactly equals one
// codebook row and rates are flat, the function must pick that row.
func TestVQWMatECPicksZeroDistortionMatch(t *testing.T) {
	cb := []int16{
		100, 200, 300, 400, 500,
		1000, -1000, 800, -200, 0,
		-500, 1500, 0, 250, -100,
	}
	cl := []int16{10, 10, 10}
	// Identity-ish W (only diagonal, scaled).
	W := []int32{
		1 << 18, 0, 0, 0, 0,
		0, 1 << 18, 0, 0, 0,
		0, 0, 1 << 18, 0, 0,
		0, 0, 0, 1 << 18, 0,
		0, 0, 0, 0, 1 << 18,
	}

	for k := int32(0); k < 3; k++ {
		in := []int16{
			cb[k*LTPOrder+0], cb[k*LTPOrder+1], cb[k*LTPOrder+2],
			cb[k*LTPOrder+3], cb[k*LTPOrder+4],
		}
		var ind, rd int32
		VQWMatECFix(&ind, &rd, in, W, cb, cl, 8, 3)
		if ind != k {
			t.Errorf("input matches row %d but got ind=%d (rd=%d)", k, ind, rd)
		}
	}
}

// TestVQWMatECRespectsRate — when two rows have identical distortion (all
// zero, since all are zero codebook), the smaller-rate one wins.
func TestVQWMatECRespectsRate(t *testing.T) {
	// Two identical rows, different rates.
	cb := []int16{
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
	}
	cl := []int16{30, 10}
	W := []int32{
		1 << 18, 0, 0, 0, 0,
		0, 1 << 18, 0, 0, 0,
		0, 0, 1 << 18, 0, 0,
		0, 0, 0, 1 << 18, 0,
		0, 0, 0, 0, 1 << 18,
	}
	in := []int16{0, 0, 0, 0, 0}

	var ind, rd int32
	VQWMatECFix(&ind, &rd, in, W, cb, cl, 8, 2)
	if ind != 1 {
		t.Errorf("rate tiebreak: got %d, want 1 (lower clQ6)", ind)
	}
}

// TestVQWMatECDeterministic — same inputs produce same outputs.
func TestVQWMatECDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	L := int32(8)
	cb := make([]int16, L*LTPOrder)
	for i := range cb {
		cb[i] = int16(rng.Intn(20001) - 10000)
	}
	cl := make([]int16, L)
	for i := range cl {
		cl[i] = int16(rng.Intn(64))
	}
	W := make([]int32, 25)
	for i := range W {
		W[i] = int32(rng.Intn(1<<19) - (1 << 18))
	}
	// Symmetrize.
	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			W[j*5+i] = W[i*5+j]
		}
	}
	in := []int16{1234, -567, 890, -123, 456}

	var ind1, rd1, ind2, rd2 int32
	VQWMatECFix(&ind1, &rd1, in, W, cb, cl, 16, L)
	VQWMatECFix(&ind2, &rd2, in, W, cb, cl, 16, L)
	if ind1 != ind2 || rd1 != rd2 {
		t.Errorf("non-deterministic: (%d,%d) vs (%d,%d)", ind1, rd1, ind2, rd2)
	}
}

// TestVQWMatECBruteForce — for L=4 and a positive-definite W, our function
// should pick the row that minimizes mu*cl + (in-cb)^T W (in-cb), which
// matches a brute-force float64 oracle. (We use a diagonal W to avoid
// reproducing the SmlaWB rounding of the cross terms.)
func TestVQWMatECBruteForceDiagonal(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	L := int32(4)
	cb := make([]int16, L*LTPOrder)
	for i := range cb {
		cb[i] = int16(rng.Intn(8001) - 4000)
	}
	cl := []int16{8, 12, 4, 16}
	// Diagonal W: only [0],[6],[12],[18],[24] non-zero.
	W := make([]int32, 25)
	W[0] = 1 << 18
	W[6] = 2 << 18
	W[12] = 1 << 17
	W[18] = 3 << 17
	W[24] = 1 << 18
	in := []int16{500, -500, 300, 1000, -200}
	muQ8 := int32(20)

	var ind int32
	var rd int32
	VQWMatECFix(&ind, &rd, in, W, cb, cl, muQ8, L)

	// Brute-force in float64 (matches the math, not the int rounding).
	var bestK int32 = -1
	bestCost := 1e30
	for k := int32(0); k < L; k++ {
		row := cb[k*LTPOrder : (k+1)*LTPOrder]
		var d [5]float64
		for i := 0; i < 5; i++ {
			d[i] = float64(in[i] - row[i])
		}
		cost := float64(muQ8) * float64(cl[k]) / 64.0
		// Diagonal quadratic form: sum_i (W_ii / 2^18) * d[i]^2
		cost += float64(W[0]) / (1 << 18) * d[0] * d[0]
		cost += float64(W[6]) / (1 << 18) * d[1] * d[1]
		cost += float64(W[12]) / (1 << 18) * d[2] * d[2]
		cost += float64(W[18]) / (1 << 18) * d[3] * d[3]
		cost += float64(W[24]) / (1 << 18) * d[4] * d[4]
		if cost < bestCost {
			bestCost = cost
			bestK = k
		}
	}
	if ind != bestK {
		t.Errorf("brute force: got ind=%d want %d (cb=%v)", ind, bestK, cb)
	}
}
