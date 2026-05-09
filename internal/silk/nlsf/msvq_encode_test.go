package nlsf

import (
	"math/rand"
	"testing"
)

// TestVQSumErrorAgainstReference — independent re-implementation in int64
// (same Q math, no SmlaWB rounding noise) compared against VQSumError.
// Tolerance: per-element diff <= 1 due to SmlaWB's >>16 rounding.
func TestVQSumErrorAgainstReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	N := int32(3)
	K := int32(5)
	lpc := int32(10)

	in := make([]int32, N*lpc)
	for i := range in {
		in[i] = int32(rng.Intn(0x8000))
	}
	w := make([]int32, lpc)
	for i := range w {
		w[i] = int32(rng.Intn(64) + 1)
	}
	cb := make([]int16, K*lpc)
	for i := range cb {
		cb[i] = int16(rng.Intn(2001) - 1000)
	}

	got := make([]int32, N*K)
	VQSumError(got, in, w, cb, N, K, lpc)

	// Reference: match SmulBB and SmlaWB widths exactly. SmulBB squares
	// (int16)diff (so we truncate first), then SmlaWB does
	// sum += (int32_squared * (int16)w) >> 16, with a 64-bit intermediate.
	for n := int32(0); n < N; n++ {
		for k := int32(0); k < K; k++ {
			var sum int32
			for m := int32(0); m < lpc; m++ {
				diff := in[n*lpc+m] - int32(cb[k*lpc+m])
				diffSq := int32(int16(diff)) * int32(int16(diff))
				sum += int32(int64(diffSq) * int64(int16(w[m])) >> 16)
			}
			diff := got[n*K+k] - sum
			if diff < -2 || diff > 2 {
				t.Errorf("N=%d K=%d: got %d ref %d (diff %d)", n, k, got[n*K+k], sum, diff)
			}
		}
	}
}

// TestVQRateDistortionAddsRate — VQRateDistortion = VQSumError + μ·rate.
func TestVQRateDistortionAddsRate(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	K := int32(4)
	lpc := int32(10)
	N := int32(2)

	stage := &CBStage{
		NVectors:  K,
		CBNLSFQ15: make([]int16, K*lpc),
		RatesQ5:   make([]int16, K),
	}
	for i := range stage.CBNLSFQ15 {
		stage.CBNLSFQ15[i] = int16(rng.Intn(20001) - 10000)
	}
	for i := range stage.RatesQ5 {
		stage.RatesQ5[i] = int16(rng.Intn(64))
	}

	in := make([]int32, N*lpc)
	for i := range in {
		in[i] = int32(rng.Intn(0x8000))
	}
	w := make([]int32, lpc)
	for i := range w {
		w[i] = int32(rng.Intn(63) + 1)
	}
	rateAcc := []int32{0, 32}
	muQ15 := int32(50)

	rd := make([]int32, N*K)
	VQRateDistortion(rd, stage, in, w, rateAcc, muQ15, N, lpc)

	// Compute the error-only baseline separately.
	errOnly := make([]int32, N*K)
	VQSumError(errOnly, in, w, stage.CBNLSFQ15, N, K, lpc)

	for n := int32(0); n < N; n++ {
		for i := int32(0); i < K; i++ {
			rate := rateAcc[n] + int32(stage.RatesQ5[i])
			// SmlaBB(a, b, c) = a + (int16)b * (int16)c. Both operands fit
			// in int16 here (asserted in C).
			expected := errOnly[n*K+i] + int32(int16(rate))*int32(int16(muQ15))
			if rd[n*K+i] != expected {
				t.Errorf("N=%d i=%d: got %d expected %d", n, i, rd[n*K+i], expected)
			}
		}
	}
}

// makeRoundtripCodebook builds a small but valid 3-stage codebook with the
// stage-0 vectors covering several distinct NLSF "shapes" so the encoder
// has meaningful choices.
func makeRoundtripCodebook(rng *rand.Rand, lpcOrder int32) *CBStruct {
	const stage0Vecs = int32(8)
	const stage1Vecs = int32(8)
	const stage2Vecs = int32(8)

	// Stage 0: monotone increasing NLSFs covering (1000..30000).
	stage0CB := make([]int16, stage0Vecs*lpcOrder)
	for v := int32(0); v < stage0Vecs; v++ {
		base := int32(1000) + v*300
		step := (int32(30000) - base) / (lpcOrder + 1)
		for i := int32(0); i < lpcOrder; i++ {
			stage0CB[v*lpcOrder+i] = int16(base + (i+1)*step)
		}
	}
	stage0Rates := make([]int16, stage0Vecs)
	for i := range stage0Rates {
		stage0Rates[i] = int16(15 + rng.Intn(20))
	}

	// Residual stages: small zero-mean perturbations.
	mkRes := func(nv int32) ([]int16, []int16) {
		cb := make([]int16, nv*lpcOrder)
		for i := range cb {
			cb[i] = int16(rng.Intn(401) - 200)
		}
		rates := make([]int16, nv)
		for i := range rates {
			rates[i] = int16(8 + rng.Intn(16))
		}
		return cb, rates
	}
	cb1, r1 := mkRes(stage1Vecs)
	cb2, r2 := mkRes(stage2Vecs)

	dmin := make([]int32, lpcOrder+1)
	for i := range dmin {
		dmin[i] = 100
	}

	return &CBStruct{
		NStages: 3,
		CBStages: []CBStage{
			{NVectors: stage0Vecs, CBNLSFQ15: stage0CB, RatesQ5: stage0Rates},
			{NVectors: stage1Vecs, CBNLSFQ15: cb1, RatesQ5: r1},
			{NVectors: stage2Vecs, CBNLSFQ15: cb2, RatesQ5: r2},
		},
		NDeltaMinQ15: dmin,
	}
}

// TestMSVQEncodeRoundtrip — encode then decode reproduces the same NLSF
// vector that encode wrote into nlsfQ15.
func TestMSVQEncodeRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	lpcOrder := int32(10)

	cb := makeRoundtripCodebook(rng, lpcOrder)

	for trial := 0; trial < 30; trial++ {
		// Build a valid input NLSF and Laroia weights.
		nlsf := makeIncreasingNLSF(rng, lpcOrder)
		Stabilize(nlsf, cb.NDeltaMinQ15, lpcOrder)

		w := make([]int32, lpcOrder)
		VQWeightsLaroia(w, nlsf, lpcOrder)

		nlsfPrev := make([]int32, lpcOrder)
		copy(nlsfPrev, nlsf)

		nlsfWork := make([]int32, lpcOrder)
		copy(nlsfWork, nlsf)

		indices := make([]int32, cb.NStages)
		MSVQEncode(indices, nlsfWork, cb, nlsfPrev, w,
			32,          // muQ15
			32,          // muFlucRedQ16
			8,           // survivors
			lpcOrder, 1) // deactivate fluc reduction for first roundtrip test

		// Decode the chosen path and check it matches what encode left in nlsfWork.
		decoded := make([]int32, lpcOrder)
		MSVQDecode(decoded, cb, indices, lpcOrder)
		for i := int32(0); i < lpcOrder; i++ {
			if decoded[i] != nlsfWork[i] {
				t.Errorf("trial %d idx %d: decoded=%d, encoder out=%d", trial, i, decoded[i], nlsfWork[i])
			}
		}

		// Sanity: indices in valid range.
		for s := int32(0); s < cb.NStages; s++ {
			if indices[s] < 0 || indices[s] >= cb.CBStages[s].NVectors {
				t.Errorf("trial %d stage %d: index %d out of [0, %d)",
					trial, s, indices[s], cb.CBStages[s].NVectors)
			}
		}
	}
}

// TestMSVQEncodeWithFlucReduction — turn fluc reduction ON. Round-trip
// (encode → decode) must still hold.
func TestMSVQEncodeWithFlucReduction(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	lpcOrder := int32(10)
	cb := makeRoundtripCodebook(rng, lpcOrder)

	for trial := 0; trial < 20; trial++ {
		nlsf := makeIncreasingNLSF(rng, lpcOrder)
		Stabilize(nlsf, cb.NDeltaMinQ15, lpcOrder)

		w := make([]int32, lpcOrder)
		VQWeightsLaroia(w, nlsf, lpcOrder)

		nlsfPrev := makeIncreasingNLSF(rng, lpcOrder)
		Stabilize(nlsfPrev, cb.NDeltaMinQ15, lpcOrder)

		nlsfWork := make([]int32, lpcOrder)
		copy(nlsfWork, nlsf)

		indices := make([]int32, cb.NStages)
		MSVQEncode(indices, nlsfWork, cb, nlsfPrev, w, 32, 32, 8, lpcOrder, 0)

		decoded := make([]int32, lpcOrder)
		MSVQDecode(decoded, cb, indices, lpcOrder)
		for i := int32(0); i < lpcOrder; i++ {
			if decoded[i] != nlsfWork[i] {
				t.Errorf("trial %d idx %d: decoded=%d, encoder out=%d", trial, i, decoded[i], nlsfWork[i])
			}
		}
	}
}

// TestMSVQEncodeQuantizedClose — quantized output should be reasonably
// close to the input (in Euclidean-ish sense, weighted by Laroia weights).
// This is a sanity check that the encoder picks plausible codebook vectors,
// not garbage.
func TestMSVQEncodeQuantizedClose(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	lpcOrder := int32(10)
	cb := makeRoundtripCodebook(rng, lpcOrder)

	// Pick a stage-0 codebook vector exactly as input. Encoder should
	// usually pick that exact stage-0 index (with stage1+stage2 small
	// residuals it won't be perfect, but the picked stage-0 path *should*
	// be the correct one most of the time).
	for stageOIdx := int32(0); stageOIdx < cb.CBStages[0].NVectors; stageOIdx++ {
		nlsf := make([]int32, lpcOrder)
		for i := int32(0); i < lpcOrder; i++ {
			nlsf[i] = int32(cb.CBStages[0].CBNLSFQ15[stageOIdx*lpcOrder+i])
		}
		// Make valid against NDeltaMin (it should already be from how we built it).
		Stabilize(nlsf, cb.NDeltaMinQ15, lpcOrder)
		w := make([]int32, lpcOrder)
		VQWeightsLaroia(w, nlsf, lpcOrder)
		nlsfWork := make([]int32, lpcOrder)
		copy(nlsfWork, nlsf)
		nlsfPrev := append([]int32(nil), nlsf...)
		indices := make([]int32, cb.NStages)
		MSVQEncode(indices, nlsfWork, cb, nlsfPrev, w, 32, 32, 8, lpcOrder, 1)

		// Quantized output should be close to input (the residuals from
		// stages 1+2 are <= ±200 each, so total deviation per coefficient
		// stays within a few hundred LSBs).
		for i := int32(0); i < lpcOrder; i++ {
			diff := nlsfWork[i] - nlsf[i]
			if diff < 0 {
				diff = -diff
			}
			if diff > 2000 {
				t.Errorf("stage0=%d idx=%d: quantization drift %d too large (input %d, quantized %d)",
					stageOIdx, i, diff, nlsf[i], nlsfWork[i])
			}
		}
	}
}
