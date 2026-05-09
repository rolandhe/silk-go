package dec

import (
	"math/rand"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
)

// TestInterpolateBoundaries — ifactQ2 = 0 → output = x0; ifactQ2 = 4 → output = x1.
func TestInterpolateBoundaries(t *testing.T) {
	x0 := []int32{100, 200, 300, 400, 500}
	x1 := []int32{1000, 2000, 3000, 4000, 5000}
	xi := make([]int32, 5)

	Interpolate(xi, x0, x1, 0, 5)
	for i := range x0 {
		if xi[i] != x0[i] {
			t.Errorf("ifact=0 idx %d: got %d want %d", i, xi[i], x0[i])
		}
	}

	Interpolate(xi, x0, x1, 4, 5)
	for i := range x1 {
		if xi[i] != x1[i] {
			t.Errorf("ifact=4 idx %d: got %d want %d", i, xi[i], x1[i])
		}
	}

	// Midpoint.
	Interpolate(xi, x0, x1, 2, 5)
	for i := range x0 {
		want := x0[i] + (x1[i]-x0[i])/2
		if xi[i] != want {
			t.Errorf("ifact=2 idx %d: got %d want %d", i, xi[i], want)
		}
	}
}

// TestBiquadIdentity — B = {1, 0, 0} in Q13, A = {0, 0} → output ≈ input.
func TestBiquadIdentity(t *testing.T) {
	const Q13 = 1 << 13
	in := []int16{100, 200, -300, 400, -500}
	out := make([]int16, len(in))
	S := make([]int32, 2)
	B := []int16{Q13, 0, 0}
	A := []int16{0, 0}
	Biquad(in, B, A, S, out, int32(len(in)))
	for i, v := range in {
		// SmlaBB introduces tiny rounding (~1 LSB).
		diff := int32(out[i]) - int32(v)
		if diff < -2 || diff > 2 {
			t.Errorf("idx %d: in=%d out=%d", i, v, out[i])
		}
	}
}

// TestBiquadAltIdentity — pass-through with BQ28 = {1<<28, 0, 0}, AQ28 = {0, 0}.
func TestBiquadAltIdentity(t *testing.T) {
	const Q28 = 1 << 28
	in := []int16{100, 200, -300, 400, -500, 600}
	out := make([]int16, len(in))
	S := make([]int32, 2)
	B := []int32{Q28, 0, 0}
	A := []int32{0, 0}
	BiquadAlt(in, B, A, S, out, int32(len(in)))
	for i, v := range in {
		diff := int32(out[i]) - int32(v)
		if diff < -2 || diff > 2 {
			t.Errorf("idx %d: in=%d out=%d", i, v, out[i])
		}
	}
}

// TestMAPredictionZero — zero coefficients → out = in (with rounding).
func TestMAPredictionZero(t *testing.T) {
	in := []int16{100, -200, 300, -400}
	out := make([]int16, len(in))
	S := make([]int32, 4)
	B := []int16{0, 0, 0, 0}
	MAPrediction(in, B, S, out, int32(len(in)), 4)
	for i, v := range in {
		if out[i] != v {
			t.Errorf("idx %d: got %d want %d", i, out[i], v)
		}
	}
}

// TestGainsQuantDequantRoundtrip — GainsQuant followed by GainsDequant
// should recover the original gain magnitudes (within Lin2Log/Log2Lin
// approximation drift).
func TestGainsQuantDequantRoundtrip(t *testing.T) {
	// Gains at known levels.
	gains := []int32{1 << 16, 4 << 16, 1 << 18, 1 << 20}
	original := append([]int32(nil), gains...)
	ind := make([]int32, 4)
	prev := int32(0)
	GainsQuant(ind, gains, &prev, 0)

	prevD := int32(0)
	dequant := make([]int32, 4)
	GainsDequant(dequant, ind, &prevD, 0)

	for i, q := range gains {
		// Both should equal the same quantized value.
		if dequant[i] != q {
			t.Errorf("idx %d: dequant=%d quantizer=%d", i, dequant[i], q)
		}
		// Quantizer ≈ original within ~2x range due to log-scale buckets.
		ratio := float64(q) / float64(original[i])
		if ratio < 0.5 || ratio > 2.0 {
			t.Errorf("idx %d: gain drift ratio %.3f (orig=%d, q=%d)", i, ratio, original[i], q)
		}
	}
}

// TestEncodeDecodeSignsRoundtrip — encode signs of pulses; decode should
// recover the original sign pattern.
func TestEncodeDecodeSignsRoundtrip(t *testing.T) {
	// Pulses with mixed signs and zeros.
	rng := rand.New(rand.NewSource(1))
	const N = 64
	pulsesIn := make([]int8, N)
	magnitudes := make([]int32, N)
	for i := 0; i < N; i++ {
		mag := int8(rng.Intn(5)) // 0..4
		if mag == 0 {
			continue
		}
		// Random sign.
		if rng.Intn(2) == 0 {
			pulsesIn[i] = -mag
		} else {
			pulsesIn[i] = mag
		}
		magnitudes[i] = int32(mag)
	}

	enc := &rangecoder.State{}
	enc.EncInit()
	EncodeSigns(enc, pulsesIn, N, 0, 0, 0)
	enc.EncWrapUp()
	_, nBytes := enc.GetLength()

	dec := &rangecoder.State{}
	dec.DecInit(enc.Buffer[:nBytes])
	pulsesOut := append([]int32(nil), magnitudes...)
	DecodeSigns(dec, pulsesOut, N, 0, 0, 0)

	for i := 0; i < N; i++ {
		if pulsesIn[i] == 0 {
			if pulsesOut[i] != 0 {
				t.Errorf("idx %d: zero pulse modified to %d", i, pulsesOut[i])
			}
			continue
		}
		if int32(pulsesIn[i]) != pulsesOut[i] {
			t.Errorf("idx %d: in=%d out=%d", i, pulsesIn[i], pulsesOut[i])
		}
	}
}

// TestShellEncoderDecoderRoundtrip — feed a 16-pulse frame through the
// shell encoder/decoder and verify recovery.
func TestShellEncoderDecoderRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 10; trial++ {
		// Build 16 non-negative pulses summing to ≤ 18 (MAX_PULSES).
		pulses := make([]int32, 16)
		total := 0
		for i := range pulses {
			if total >= 16 {
				break
			}
			v := rng.Intn(3)
			if total+v > 16 {
				v = 16 - total
			}
			pulses[i] = int32(v)
			total += v
		}

		enc := &rangecoder.State{}
		enc.EncInit()
		ShellEncoder(enc, pulses)
		enc.EncWrapUp()
		_, nBytes := enc.GetLength()

		dec := &rangecoder.State{}
		dec.DecInit(enc.Buffer[:nBytes])
		decoded := make([]int32, 16)
		ShellDecoder(decoded, dec, int32(total))

		for i, v := range pulses {
			if decoded[i] != v {
				t.Errorf("trial %d idx %d: got %d want %d", trial, i, decoded[i], v)
			}
		}
	}
}

// TestDecodePitch24kHz — known mapping for 24 kHz.
func TestDecodePitch24kHz(t *testing.T) {
	pitchLags := make([]int32, 4)
	DecodePitch(50, 0, pitchLags, 24)
	// minLag = 2*24 = 48; lag = 48+50 = 98; pitchLags = lag + CB_lags_stage3[i][0].
	for i := int32(0); i < 4; i++ {
		// All four should be derived from the same `lag` plus a row-specific offset.
		if pitchLags[i] < 0 {
			t.Errorf("subfr %d: negative pitch %d", i, pitchLags[i])
		}
	}
}

// TestDecodePitch8kHzPath — 8 kHz uses stage2 (smaller) codebook.
func TestDecodePitch8kHzPath(t *testing.T) {
	pitchLags := make([]int32, 4)
	DecodePitch(20, 5, pitchLags, 8)
	// minLag = 2*8 = 16; lag = 16+20 = 36.
	for i, p := range pitchLags {
		if p < 0 {
			t.Errorf("subfr %d: negative pitch %d", i, p)
		}
	}
}
