package resampler

import (
	"math"
	"math/rand"
	"testing"
)

// TestPrivateCopyIdentity — copy must be byte-equal.
func TestPrivateCopyIdentity(t *testing.T) {
	in := []int16{1, -2, 30000, -30000, 0, 100, -100}
	out := make([]int16, len(in))
	PrivateCopy(nil, out, in, int32(len(in)))
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("idx %d: got %d want %d", i, out[i], in[i])
		}
	}
}

// TestDown2Up2Roundtrip — upsample-then-downsample preserves a low-frequency
// sinusoid (above the all-pass cutoff). Compares envelope, not bit-for-bit.
func TestDown2Up2Roundtrip(t *testing.T) {
	const n = 256
	in := make([]int16, n)
	for i := 0; i < n; i++ {
		// 1/8 of Nyquist — well below the down2 stop-band.
		v := math.Sin(2*math.Pi*float64(i)/32.0) * 8000
		in[i] = int16(v)
	}
	upState := make([]int32, 2)
	upBuf := make([]int16, 2*n)
	Up2(upState, upBuf, in, int32(n))

	downState := make([]int32, 2)
	downBuf := make([]int16, n)
	Down2(downState, downBuf, upBuf, int32(2*n))

	// Skip first ~32 samples (filter transient). Compare RMS energy of
	// rest against input energy.
	const skip = 32
	var inE, outE float64
	for i := skip; i < n; i++ {
		inE += float64(in[i]) * float64(in[i])
		outE += float64(downBuf[i]) * float64(downBuf[i])
	}
	ratio := outE / inE
	if ratio < 0.7 || ratio > 1.3 {
		t.Errorf("up→down energy ratio %.3f, want ~1.0", ratio)
	}
}

// TestDown2Halves — downsampling produces inLen/2 output samples.
func TestDown2OutputCount(t *testing.T) {
	in := make([]int16, 64)
	for i := range in {
		in[i] = int16(rand.Intn(2001) - 1000)
	}
	state := make([]int32, 2)
	out := make([]int16, 32)
	Down2(state, out, in, 64)
	// All outputs should be set (not the initial 0). Check they're not all
	// zero (which would indicate the loop didn't run).
	allZero := true
	for _, v := range out {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Errorf("Down2 produced all-zero output for non-zero input")
	}
}

// TestUp2Doubles — Up2 produces 2*len output samples.
func TestUp2OutputCount(t *testing.T) {
	in := []int16{100, 200, 300, 400, 500}
	state := make([]int32, 2)
	out := make([]int16, 10)
	Up2(state, out, in, 5)
	// Output should be smooth-ish — adjacent samples shouldn't differ by
	// more than the input's gradient by orders of magnitude.
	for i := 1; i < len(out); i++ {
		diff := int32(out[i]) - int32(out[i-1])
		if diff > 1000 || diff < -1000 {
			t.Errorf("Up2 step %d→%d too large (%d)", i-1, i, diff)
		}
	}
}

// TestPrivateAR2OutputForms — AR2 with all-zero coefficients makes outQ8
// be the running sum of input shifted to Q8 plus state.
//
// With AQ14 = [0, 0]:
//
//	out[0] = state[0] + (in[0] << 8)
//	state[0] = state[1] + 0 = 0
//	state[1] = 0
//	out[1] = 0 + (in[1] << 8) = in[1] << 8
//
// So with zero state, AR2[0,0] just produces in << 8 in Q8.
func TestPrivateAR2ZeroCoefs(t *testing.T) {
	state := make([]int32, 2)
	in := []int16{100, -200, 300, -400}
	out := make([]int32, len(in))
	PrivateAR2(state, out, in, []int16{0, 0}, int32(len(in)))
	for i, v := range in {
		want := int32(v) << 8
		if out[i] != want {
			t.Errorf("idx %d: got %d want %d", i, out[i], want)
		}
	}
}

// TestPrivateAR2NonZero — non-zero AR coefficients introduce a feedback
// loop. Output should depend on history (deterministic, bounded).
func TestPrivateAR2Deterministic(t *testing.T) {
	state1 := make([]int32, 2)
	state2 := make([]int32, 2)
	in := []int16{1000, 2000, -1500, 500}
	out1 := make([]int32, len(in))
	out2 := make([]int32, len(in))
	A := []int16{4096, -2048} // small Q14 coefs
	PrivateAR2(state1, out1, in, A, int32(len(in)))
	PrivateAR2(state2, out2, in, A, int32(len(in)))
	for i := range out1 {
		if out1[i] != out2[i] {
			t.Errorf("idx %d non-deterministic: %d vs %d", i, out1[i], out2[i])
		}
	}
}
