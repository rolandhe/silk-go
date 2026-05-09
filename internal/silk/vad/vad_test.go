package vad

import (
	"math"
	"testing"
)

// TestInitDefaults — VADInit zeros the state, seeds Counter to 15, and
// produces strictly decreasing per-band noise-level biases (pink-noise
// approximation: psd ∝ 1/f).
func TestInitDefaults(t *testing.T) {
	var s State
	s.HPstate = 1234 // dirty value to verify zeroing.
	s.Counter = 999

	if rc := Init(&s); rc != 0 {
		t.Fatalf("Init returned %d", rc)
	}
	if s.HPstate != 0 {
		t.Errorf("HPstate not zeroed: %d", s.HPstate)
	}
	if s.Counter != 15 {
		t.Errorf("Counter = %d, want 15", s.Counter)
	}
	for b := 0; b < 4; b++ {
		if s.NoiseLevelBias[b] <= 0 {
			t.Errorf("band %d: NoiseLevelBias %d must be positive", b, s.NoiseLevelBias[b])
		}
		if s.NL[b] <= 0 || s.InvNL[b] <= 0 {
			t.Errorf("band %d: NL=%d InvNL=%d", b, s.NL[b], s.InvNL[b])
		}
		if s.NrgRatioSmthQ8[b] != 100*256 {
			t.Errorf("band %d: NrgRatioSmthQ8 = %d, want 25600", b, s.NrgRatioSmthQ8[b])
		}
	}
	// Pink noise: NoiseLevelBias should not increase with band index.
	for b := 1; b < 4; b++ {
		if s.NoiseLevelBias[b] > s.NoiseLevelBias[b-1] {
			t.Errorf("NoiseLevelBias not monotone non-increasing: %v", s.NoiseLevelBias)
		}
	}
}

// TestSilenceLowSA — feeding pure silence must keep the SA estimate small.
func TestSilenceLowSA(t *testing.T) {
	var s State
	Init(&s)

	const frameLen = 320 // 16 kHz × 20 ms
	in := make([]int16, frameLen)
	quality := make([]int32, 4)

	var sa, snr, tilt int32
	// Run a few frames so the smoothers settle.
	for f := 0; f < 5; f++ {
		GetSAQ8(&s, &sa, &snr, &tilt, quality, in, frameLen)
	}
	if sa > 32 { // Q8 → ~12% of max
		t.Errorf("silence: SA_Q8 = %d, want low value", sa)
	}
}

// TestLoudToneHighSA — feeding a strong sinusoid at ~1 kHz should drive the
// SA estimate well above the silence floor.
func TestLoudToneHighSA(t *testing.T) {
	var sSilence State
	Init(&sSilence)
	var sTone State
	Init(&sTone)

	const frameLen = 320
	in := make([]int16, frameLen)
	tone := make([]int16, frameLen)
	for i := 0; i < frameLen; i++ {
		// 1 kHz sine at 16 kHz sample rate, ~half-scale.
		tone[i] = int16(16000 * math.Sin(2*math.Pi*1000*float64(i)/16000))
	}
	quality := make([]int32, 4)

	var saS, saT, snr, tilt int32
	for f := 0; f < 8; f++ {
		GetSAQ8(&sSilence, &saS, &snr, &tilt, quality, in, frameLen)
		GetSAQ8(&sTone, &saT, &snr, &tilt, quality, tone, frameLen)
	}
	if saT <= saS {
		t.Errorf("tone SA (%d) should exceed silence SA (%d)", saT, saS)
	}
	if saT < 64 { // Q8 → ~25% of max
		t.Errorf("loud tone SA_Q8 = %d, want >= 64", saT)
	}
}

// TestNoiseLevelTracksFloor — running a steady moderate-energy signal for
// many frames should pull NL upward from its initial seed.
func TestNoiseLevelTracksFloor(t *testing.T) {
	var s State
	Init(&s)
	const frameLen = 320
	noise := make([]int16, frameLen)
	for i := range noise {
		noise[i] = int16((i*1103515245 + 12345) & 0x3FF) // small white-ish
	}
	quality := make([]int32, 4)
	var sa, snr, tilt int32

	startNL := s.NL
	for f := 0; f < 200; f++ {
		GetSAQ8(&s, &sa, &snr, &tilt, quality, noise, frameLen)
	}

	// At least one band should have grown above its initialized value.
	moved := false
	for b := 0; b < 4; b++ {
		if s.NL[b] > startNL[b] {
			moved = true
			break
		}
	}
	if !moved {
		t.Errorf("NL did not adapt: start=%v end=%v", startNL, s.NL)
	}
}
