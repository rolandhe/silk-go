package nlsf

import (
	"math/rand"
	"testing"
)

// makeTestCodebook builds a small valid MSVQ codebook with known-valid
// stage-0 vectors and small residual stages.
func makeTestCodebook(rng *rand.Rand, lpcOrder int32, nStages int32) (*CBStruct, []int32) {
	stages := make([]CBStage, nStages)

	// Stage 0: produce 4 monotone increasing NLSF vectors.
	stage0Vecs := int32(4)
	stage0CB := make([]int16, stage0Vecs*lpcOrder)
	for v := int32(0); v < stage0Vecs; v++ {
		// Spread NLSFs uniformly across (1000, 30000).
		base := int32(1000) + v*200
		step := (int32(30000) - base) / (lpcOrder + 1)
		for i := int32(0); i < lpcOrder; i++ {
			stage0CB[v*lpcOrder+i] = int16(base + (i+1)*step)
		}
	}
	stages[0] = CBStage{NVectors: stage0Vecs, CBNLSFQ15: stage0CB}

	// Stages 1..n: small zero-mean residuals.
	for s := int32(1); s < nStages; s++ {
		nv := int32(4)
		cb := make([]int16, nv*lpcOrder)
		for v := int32(0); v < nv; v++ {
			for i := int32(0); i < lpcOrder; i++ {
				cb[v*lpcOrder+i] = int16(rng.Intn(201) - 100) // ±100 in Q15
			}
		}
		stages[s] = CBStage{NVectors: nv, CBNLSFQ15: cb}
	}

	// NDeltaMin: 100 LSBs spacing.
	dmin := make([]int32, lpcOrder+1)
	for i := range dmin {
		dmin[i] = 100
	}

	return &CBStruct{
		NStages:      nStages,
		CBStages:     stages,
		NDeltaMinQ15: dmin,
	}, dmin
}

// TestMSVQDecodeStageOnlyMatchesStage0 — selecting stage 0 with a single-stage
// codebook reproduces the stage 0 codebook vector exactly (after Stabilize).
func TestMSVQDecodeStage0Only(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	lpcOrder := int32(10)
	cb, _ := makeTestCodebook(rng, lpcOrder, 1)

	indices := []int32{2}
	out := make([]int32, lpcOrder)
	MSVQDecode(out, cb, indices, lpcOrder)

	// Compare with the raw stage-0 vector after Stabilize would treat it as-is
	// (since we made it monotonically increasing with > 100 spacing).
	row := cb.CBStages[0].CBNLSFQ15[indices[0]*lpcOrder : (indices[0]+1)*lpcOrder]
	for i := int32(0); i < lpcOrder; i++ {
		if out[i] != int32(row[i]) {
			t.Errorf("idx %d: got %d want %d", i, out[i], int32(row[i]))
		}
	}
}

// TestMSVQDecodeAddsResiduals — multi-stage decode is sum of stages, then
// stabilize. Verify the sum semantics by constructing codebooks where the
// expected output is predictable.
func TestMSVQDecodeMultiStage(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	lpcOrder := int32(10)
	cb, _ := makeTestCodebook(rng, lpcOrder, 3)

	indices := []int32{1, 0, 2}
	out := make([]int32, lpcOrder)
	MSVQDecode(out, cb, indices, lpcOrder)

	// Independently compute the un-stabilized sum.
	expected := make([]int32, lpcOrder)
	for s := int32(0); s < cb.NStages; s++ {
		off := indices[s] * lpcOrder
		for i := int32(0); i < lpcOrder; i++ {
			expected[i] += int32(cb.CBStages[s].CBNLSFQ15[off+i])
		}
	}
	// Apply our own Stabilize on the expected vector (same routine the
	// production decoder uses) and compare.
	Stabilize(expected, cb.NDeltaMinQ15, lpcOrder)
	for i := int32(0); i < lpcOrder; i++ {
		if out[i] != expected[i] {
			t.Errorf("idx %d: got %d want %d (sum-then-stabilize)", i, out[i], expected[i])
		}
	}
}

// TestMSVQDecodeOutputAlwaysValid — random index selection produces a stable,
// sorted NLSF (this is the contract Stabilize should ensure).
func TestMSVQDecodeOutputSorted(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	lpcOrder := int32(10)
	cb, _ := makeTestCodebook(rng, lpcOrder, 4)

	for trial := 0; trial < 50; trial++ {
		indices := make([]int32, cb.NStages)
		for s := int32(0); s < cb.NStages; s++ {
			indices[s] = int32(rng.Intn(int(cb.CBStages[s].NVectors)))
		}
		out := make([]int32, lpcOrder)
		MSVQDecode(out, cb, indices, lpcOrder)

		for i := int32(1); i < lpcOrder; i++ {
			if out[i] < out[i-1] {
				t.Errorf("trial %d: out not sorted at %d: %v", trial, i, out)
				break
			}
		}
		if out[0] < cb.NDeltaMinQ15[0] {
			t.Errorf("trial %d: out[0]=%d < dmin[0]=%d", trial, out[0], cb.NDeltaMinQ15[0])
		}
		if out[lpcOrder-1] > (1<<15)-cb.NDeltaMinQ15[lpcOrder] {
			t.Errorf("trial %d: out[last]=%d too large", trial, out[lpcOrder-1])
		}
	}
}

// TestMSVQDecodeOrder16 — same as multi-stage test but at LPC order 16,
// matches the unrolled lpc_order==16 path of the C reference.
func TestMSVQDecodeOrder16(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	lpcOrder := int32(16)
	cb, _ := makeTestCodebook(rng, lpcOrder, 3)
	indices := []int32{2, 1, 3}
	out := make([]int32, lpcOrder)
	MSVQDecode(out, cb, indices, lpcOrder)

	expected := make([]int32, lpcOrder)
	for s := int32(0); s < cb.NStages; s++ {
		off := indices[s] * lpcOrder
		for i := int32(0); i < lpcOrder; i++ {
			expected[i] += int32(cb.CBStages[s].CBNLSFQ15[off+i])
		}
	}
	Stabilize(expected, cb.NDeltaMinQ15, lpcOrder)
	for i := int32(0); i < lpcOrder; i++ {
		if out[i] != expected[i] {
			t.Errorf("order16 idx %d: got %d want %d", i, out[i], expected[i])
		}
	}
}
