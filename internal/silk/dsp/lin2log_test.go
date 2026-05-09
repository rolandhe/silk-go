package dsp

import (
	"math"
	"testing"
)

// TestLin2LogVsFloat — Lin2Log returns ~128*log2(inLin) in Q7.
func TestLin2LogVsFloat(t *testing.T) {
	for _, x := range []int32{1, 10, 1000, 1 << 15, 1 << 24, 0x7FFFFFFF} {
		got := Lin2Log(x)
		want := 128 * math.Log2(float64(x))
		gotF := float64(got)
		// Tolerance ~3 LSB in Q7 due to the parabolic approximation.
		if math.Abs(gotF-want) > 3 {
			t.Errorf("Lin2Log(%d) = %d (real %.2f), want ~%.2f", x, got, gotF, want)
		}
	}
}

// TestLog2LinVsFloat — Log2Lin is the inverse: ~2^(in/128).
func TestLog2LinVsFloat(t *testing.T) {
	for _, q := range []int32{0, 128, 256, 1024, 2048, 30 << 7} {
		got := Log2Lin(q)
		want := math.Pow(2, float64(q)/128.0)
		// Both Lin2Log and Log2Lin are piecewise-parabolic; accept up to
		// 0.5% relative error.
		if math.Abs(float64(got)-want) > want*0.005+1 {
			t.Errorf("Log2Lin(%d) = %d, want ~%.2f", q, got, want)
		}
	}
}

// TestLin2LogLog2LinRoundtrip — composition is approximately identity.
func TestLin2LogLog2LinRoundtrip(t *testing.T) {
	for _, x := range []int32{1, 100, 10000, 1 << 20, 1 << 28} {
		round := Log2Lin(Lin2Log(x))
		// Two parabolic approximations stacked → ~1.5% worst case.
		diff := math.Abs(float64(round-x)) / float64(x)
		if diff > 0.02 {
			t.Errorf("roundtrip(%d) = %d (rel %.4f)", x, round, diff)
		}
	}
}

// TestLog2LinClamps — saturation at both ends.
func TestLog2LinClamps(t *testing.T) {
	if Log2Lin(-1) != 0 {
		t.Errorf("Log2Lin(-1) want 0")
	}
	if Log2Lin(31<<7) != 0x7FFFFFFF {
		t.Errorf("Log2Lin(31<<7) want int32_max")
	}
}
