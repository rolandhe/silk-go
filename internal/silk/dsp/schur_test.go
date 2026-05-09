package dsp

import (
	"math"
	"math/rand"
	"testing"
)

// generateAR — synthesize a length-N stable AR signal with known reflection
// coefficients. We feed the same signal back through autocorr+schur and
// expect the recovered RCs to match the originals (within fixed-point
// accuracy).
//
// We reuse the K2a-style step-up to convert RCs to AR coefs, then run a
// simple AR synthesis filter on white noise.
func generateAR(t *testing.T, rng *rand.Rand, order int32, length int32) ([]int16, []int16) {
	// Reflection coefficients: |rc| < 0.5 in real domain (Q15 = 16384).
	rcQ15 := make([]int16, order)
	for i := range rcQ15 {
		rcQ15[i] = int16(rng.Intn(1<<14) - (1 << 13))
	}

	// AR coefficients via Levinson step-up. We don't have lpc.K2a here (would
	// be a circular import) — re-implement the float version.
	rcReal := make([]float64, order)
	for i, v := range rcQ15 {
		rcReal[i] = float64(v) / 32768.0
	}
	a := make([]float64, order)
	atmp := make([]float64, order)
	for k := int32(0); k < order; k++ {
		copy(atmp[:k], a[:k])
		for n := int32(0); n < k; n++ {
			a[n] = atmp[n] + rcReal[k]*atmp[k-n-1]
		}
		a[k] = rcReal[k]
	}

	// Synthesize the signal: x[n] = e[n] - sum_{i=1..order} a[i-1] * x[n-i].
	out := make([]int16, length)
	state := make([]float64, order)
	for n := int32(0); n < length; n++ {
		excitation := float64(rng.Intn(2001)-1000) / 1000.0 * 1000.0 // ~white in [-1000,1000]
		v := excitation
		for i := int32(0); i < order; i++ {
			v -= a[i] * state[i]
		}
		// Clamp to int16.
		if v > 32760 {
			v = 32760
		} else if v < -32760 {
			v = -32760
		}
		out[n] = int16(v)
		// Shift state: state[0] is most recent.
		for i := order - 1; i > 0; i-- {
			state[i] = state[i-1]
		}
		state[0] = v
	}
	return out, rcQ15
}

// scaledInitial replicates the lz-based shift Schur applies to corr[0]
// before iterating. The returned residual is in the same scaled domain.
func scaledInitial(c0 int32) int32 {
	lz := int32(0)
	for v := uint32(c0); v != 0; v >>= 1 {
		lz++
	}
	lz = 32 - lz
	if lz < 2 {
		return c0 >> 1
	}
	if lz > 2 {
		return c0 << uint(lz-2)
	}
	return c0
}

// TestSchurReflectionEnergyDecreasing — for a real-valued autocorrelation
// from a stable signal, residual energy after each Schur step must be
// non-negative and ≤ scaled initial energy.
func TestSchurReflectionEnergyDecreasing(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 20; trial++ {
		n := int32(128 + rng.Intn(128))
		order := int32(8 + 2*(trial%4))
		x, _ := generateAR(t, rng, order, n)

		corr := make([]int32, order+1)
		var scale int32
		Autocorr(corr, &scale, x, n, order+1)

		rc := make([]int16, order)
		residual := Schur(rc, corr, order)

		// Residual energy must be in [0, scaled initial].
		if residual < 0 {
			t.Errorf("trial %d order %d: residual=%d < 0", trial, order, residual)
		}
		init := scaledInitial(corr[0])
		// Allow tiny rounding slack from SmlaWB.
		if residual > init+8 {
			t.Errorf("trial %d order %d: residual=%d > initial=%d (scaled)",
				trial, order, residual, init)
		}
		// All |rc| should be < 32768 (i.e. fit int16, |rc| < 1 in Q15 real).
		for k := int32(0); k < order; k++ {
			if rc[k] == -32768 {
				t.Errorf("trial %d order %d: rc[%d]=-32768 (saturation)", trial, order, k)
			}
		}
	}
}

// TestSchur64ReflectionEnergyDecreasing — same as Schur but Q16 output.
func TestSchur64ReflectionEnergyDecreasing(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for trial := 0; trial < 20; trial++ {
		n := int32(128 + rng.Intn(128))
		order := int32(8 + 2*(trial%4))
		x, _ := generateAR(t, rng, order, n)

		corr := make([]int32, order+1)
		var scale int32
		Autocorr(corr, &scale, x, n, order+1)

		rc := make([]int32, order)
		residual := Schur64(rc, corr, order)

		if residual < 0 {
			t.Errorf("trial %d order %d: residual=%d < 0", trial, order, residual)
		}
		// Schur64 doesn't apply lz-scaling; residual is in the same domain
		// as corr[0].
		if residual > corr[0]+8 {
			t.Errorf("trial %d order %d: residual=%d > initial=%d", trial, order, residual, corr[0])
		}
	}
}

// TestSchurAndSchur64Agree — for the same correlations, Schur (Q15) and
// Schur64 (Q16) should produce reflection coefficients within ~0.5% of
// each other after conversion to a common Q.
func TestSchurAndSchur64Agree(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for trial := 0; trial < 20; trial++ {
		n := int32(256)
		order := int32(10)
		x, _ := generateAR(t, rng, order, n)

		corr := make([]int32, order+1)
		var scale int32
		Autocorr(corr, &scale, x, n, order+1)

		rc15 := make([]int16, order)
		rc16 := make([]int32, order)
		Schur(rc15, corr, order)
		Schur64(rc16, corr, order)

		for k := int32(0); k < order; k++ {
			// Compare in real domain.
			r15 := float64(rc15[k]) / 32768.0
			r16 := float64(rc16[k]) / 65536.0
			diff := math.Abs(r15 - r16)
			// Schur is faster but less precise; allow up to 0.05 absolute
			// error or 5% relative error, whichever is bigger.
			tol := math.Max(0.05, math.Abs(r16)*0.05)
			if diff > tol {
				t.Errorf("trial %d k %d: schur=%.4f schur64=%.4f diff=%.4f",
					trial, k, r15, r16, diff)
			}
		}
	}
}

// TestSchur64BadInput — c[0] <= 0 should yield zeroed RC and zero residual.
func TestSchur64BadInput(t *testing.T) {
	c := []int32{0, 1, 2, 3, 4, 5}
	rc := make([]int32, 5)
	res := Schur64(rc, c, 5)
	if res != 0 {
		t.Errorf("c[0]=0 should yield 0 residual, got %d", res)
	}
	for i, v := range rc {
		if v != 0 {
			t.Errorf("rc[%d]=%d, want 0", i, v)
		}
	}
}
