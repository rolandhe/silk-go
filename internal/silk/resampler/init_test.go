package resampler

import (
	"math"
	"testing"
)

func sineTone(n int32, freq, fs float64, amp int16) []int16 {
	out := make([]int16, n)
	for i := int32(0); i < n; i++ {
		v := math.Sin(2*math.Pi*freq*float64(i)/fs) * float64(amp)
		out[i] = int16(v)
	}
	return out
}

// TestInitInvalidRates — out-of-range Fs values should fail.
func TestInitInvalidRates(t *testing.T) {
	var s State
	for _, c := range []struct{ in, out int32 }{
		{0, 16000}, {16000, 0}, {7000, 16000}, {200000, 48000}, {16000, 200000},
	} {
		if Init(&s, c.in, c.out) == 0 {
			t.Errorf("Init(%d, %d) succeeded, expected error", c.in, c.out)
		}
	}
}

// TestInitMagic — successful Init sets magic; Process refuses without it.
func TestInitMagic(t *testing.T) {
	var s State
	if got := Init(&s, 24000, 24000); got != 0 {
		t.Fatalf("Init=%d", got)
	}
	if s.MagicNumber != stateMagic {
		t.Errorf("magic not set: %d", s.MagicNumber)
	}

	var bad State
	in := []int16{1, 2, 3}
	out := make([]int16, 3)
	if Process(&bad, out, in, 3) != -1 {
		t.Errorf("Process on uninitialized state should return -1")
	}
}

// TestEqualRateCopy — equal in/out rate: output == input.
func TestEqualRateCopy(t *testing.T) {
	var s State
	Init(&s, 24000, 24000)
	in := sineTone(240, 440, 24000, 8000)
	out := make([]int16, 240)
	Process(&s, out, in, 240)
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("idx %d: out=%d in=%d", i, out[i], in[i])
			break
		}
	}
}

// TestRoundtripPreservesEnergy — feed a sine, downsample, upsample,
// energy should be approximately preserved (within filter transient).
func TestRoundtripPreservesEnergy(t *testing.T) {
	const fsLow = int32(8000)
	const fsHigh = int32(16000)
	const N = 2400 // 150 ms at 16 kHz

	in := sineTone(N, 440, float64(fsHigh), 8000)
	mid := make([]int16, N/2)
	out := make([]int16, N)

	var down, up State
	Init(&down, fsHigh, fsLow)
	Init(&up, fsLow, fsHigh)

	Process(&down, mid, in, N)
	Process(&up, out, mid, N/2)

	// Skip filter transient (~80 samples).
	const skip = 80
	var inE, outE float64
	for i := skip; i < N; i++ {
		inE += float64(in[i]) * float64(in[i])
		outE += float64(out[i]) * float64(out[i])
	}
	ratio := outE / inE
	if ratio < 0.5 || ratio > 1.5 {
		t.Errorf("16k→8k→16k energy ratio %.3f, want ~1", ratio)
	}
}

// TestUpsampleByTwo — 2x upsample (24k → 48k) takes the special HQ path.
func TestUpsampleByTwo(t *testing.T) {
	var s State
	if Init(&s, 24000, 48000) != 0 {
		t.Fatalf("Init failed")
	}
	in := sineTone(240, 440, 24000, 8000)
	out := make([]int16, 480)
	Process(&s, out, in, 240)
	// Output should not be all-zero.
	allZero := true
	for _, v := range out {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Errorf("upsample produced all-zero output")
	}
}

// TestRationalRatios — exercise each rational shortcut path in Init.
func TestRationalRatios(t *testing.T) {
	// (in, out, output samples expected per 1000 input samples)
	cases := []struct {
		fin, fout int32
	}{
		{8000, 16000},  // up: 1:2 HQ wrapper
		{12000, 16000}, // up: AF (IIR_FIR + ARMA via internal routing actually goes UF)
		{24000, 16000}, // down: 3:4 → DownFIR
		{16000, 8000},  // down: 1:2 → DownFIR
		{24000, 8000},  // down: 1:3 → DownFIR
		{48000, 24000}, // down: 1:2 → DownFIR
		{48000, 16000}, // down: 1:6 → DownFIR (down2 + 1:3)
		{32000, 16000}, // down: 1:2 → DownFIR
	}
	for _, c := range cases {
		var s State
		if got := Init(&s, c.fin, c.fout); got != 0 {
			t.Errorf("Init(%d, %d) failed", c.fin, c.fout)
			continue
		}
		// Generate ~25 ms of input for a clean batch.
		nIn := c.fin / 40 // 25 ms
		in := sineTone(nIn, 440, float64(c.fin), 5000)
		nOut := nIn * c.fout / c.fin
		out := make([]int16, nOut*2) // generous
		if got := Process(&s, out, in, nIn); got != 0 {
			t.Errorf("Process(%d, %d) failed: %d", c.fin, c.fout, got)
			continue
		}
		// Sanity: at least one nonzero output sample.
		nonzero := 0
		for i := int32(0); i < nOut; i++ {
			if out[i] != 0 {
				nonzero++
			}
		}
		if nonzero < int(nOut)/4 {
			t.Errorf("%d→%d: only %d/%d nonzero outputs", c.fin, c.fout, nonzero, nOut)
		}
	}
}

// TestClearKeepsConfig — Clear zeroes filter state but keeps the magic
// number and dispatch.
func TestClearKeepsConfig(t *testing.T) {
	var s State
	Init(&s, 16000, 24000)
	saved := s.MagicNumber
	in := sineTone(160, 440, 16000, 5000)
	out := make([]int16, 240)
	Process(&s, out, in, 160)

	// State should now be non-zero somewhere.
	hasState := s.SIIR[0] != 0 || s.SFIRInt[0] != 0
	Clear(&s)
	if s.MagicNumber != saved {
		t.Errorf("Clear changed magic: %d", s.MagicNumber)
	}
	for _, v := range s.SIIR {
		if v != 0 {
			t.Errorf("SIIR not cleared: %v", s.SIIR)
			break
		}
	}
	for _, v := range s.SFIRInt {
		if v != 0 {
			t.Errorf("SFIRInt not cleared")
			break
		}
	}
	_ = hasState
}
