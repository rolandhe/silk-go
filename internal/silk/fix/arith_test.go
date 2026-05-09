package fix

import (
	"math"
	"testing"
)

func TestSat16(t *testing.T) {
	cases := []struct {
		in   int32
		want int32
	}{
		{0, 0},
		{int16Max, int16Max},
		{int16Min, int16Min},
		{int16Max + 1, int16Max},
		{int16Min - 1, int16Min},
		{1 << 30, int16Max},
		{-(1 << 30), int16Min},
	}
	for _, c := range cases {
		if got := Sat16(c.in); got != c.want {
			t.Errorf("Sat16(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestAddPosSat32(t *testing.T) {
	if got := AddPosSat32(1<<30, 1<<30); got != int32Max {
		t.Errorf("AddPosSat32 overflow case: got %d", got)
	}
	if got := AddPosSat32(100, 200); got != 300 {
		t.Errorf("AddPosSat32 normal case: got %d", got)
	}
}

func TestAddSubSat32(t *testing.T) {
	if got := AddSat32(int32Max, 1); got != int32Max {
		t.Errorf("AddSat32 overflow: %d", got)
	}
	if got := AddSat32(int32Min, -1); got != int32Min {
		t.Errorf("AddSat32 underflow: %d", got)
	}
	if got := SubSat32(int32Max, -1); got != int32Max {
		t.Errorf("SubSat32 overflow: %d", got)
	}
	if got := SubSat32(int32Min, 1); got != int32Min {
		t.Errorf("SubSat32 underflow: %d", got)
	}
}

func TestRShiftRound(t *testing.T) {
	if got := RShiftRound(7, 1); got != 4 {
		t.Errorf("RShiftRound(7,1) = %d", got)
	}
	if got := RShiftRound(8, 2); got != 2 {
		t.Errorf("RShiftRound(8,2) = %d", got)
	}
	if got := RShiftRound(-7, 1); got != -3 {
		t.Errorf("RShiftRound(-7,1) = %d", got)
	}
	// shift > 1
	if got := RShiftRound(15, 3); got != 2 {
		t.Errorf("RShiftRound(15,3) = %d", got)
	}
}

func TestLShiftOvflw32Wraps(t *testing.T) {
	// 1<<31 wraps to int32Min
	if got := LShiftOvflw32(1, 31); got != int32Min {
		t.Errorf("LShiftOvflw32(1,31) = %d, want %d", got, int32Min)
	}
	if got := LShiftOvflw32(int32Min, 1); got != 0 {
		t.Errorf("LShiftOvflw32(int32Min,1) = %d, want 0", got)
	}
}

func TestLShiftSat32(t *testing.T) {
	cases := []struct{ a, sh, want int32 }{
		{1, 4, 16},
		// SKP_LSHIFT_SAT32 result of saturating-shift is clamp-then-shift,
		// so the saturated value at sh=1 is (INT32_MAX >> 1) << 1 = INT32_MAX-1,
		// not INT32_MAX. Verified against the C oracle.
		{int32Max, 1, int32Max - 1},
		{int32Min, 1, int32Min},
		{1 << 14, 12, 1 << 14 << 12},
	}
	for _, c := range cases {
		if got := LShiftSat32(c.a, c.sh); got != c.want {
			t.Errorf("LShiftSat32(%d,%d) = %d, want %d", c.a, c.sh, got, c.want)
		}
	}
}

func TestAddOvflw32(t *testing.T) {
	if got := AddOvflw32(int32Max, 1); got != int32Min {
		t.Errorf("AddOvflw32 max+1 = %d, want %d", got, int32Min)
	}
	if got := SubOvflw32(int32Min, 1); got != int32Max {
		t.Errorf("SubOvflw32 min-1 = %d, want %d", got, int32Max)
	}
}

func TestMulMlaSeries(t *testing.T) {
	if got := Mla(10, 3, 4); got != 22 {
		t.Errorf("Mla = %d", got)
	}
	// int32Max + 1*2 wraps to int32Max + 2 - 2^32 = -int32Max
	if got := MlaOvflw(int32Max, 1, 2); got != -int32Max {
		t.Errorf("MlaOvflw wrap: got %d", got)
	}
}

// SmulBB — must match (int16)(a) * (int16)(b).
func TestSmulBB(t *testing.T) {
	cases := []struct{ a, b int32 }{
		{0x12345678, 0x7FFF0001},
		{-1, 1},
		{0x00008000, 0x00008000}, // (int16)0x8000 = -32768
		{32767, 32767},
	}
	for _, c := range cases {
		want := int32(int16(c.a)) * int32(int16(c.b))
		if got := SmulBB(c.a, c.b); got != want {
			t.Errorf("SmulBB(%#x,%#x)=%d want %d", c.a, c.b, got, want)
		}
	}
}

// SmulWB — match the C macro semantics exactly.
func TestSmulWB(t *testing.T) {
	cases := []struct{ a, b int32 }{
		{0x12345678, 0x7FFF},
		{-1, -1},
		{0x40000000, 0x4000},
		{1 << 28, 1234},
		{int32Max, -1},
	}
	for _, c := range cases {
		bs := int32(int16(c.b))
		want := (c.a>>16)*bs + ((c.a&0xFFFF)*bs)>>16
		if got := SmulWB(c.a, c.b); got != want {
			t.Errorf("SmulWB(%#x,%#x)=%d want %d", c.a, c.b, got, want)
		}
	}
}

// SmulWW — verifies the macro identity on representative inputs.
func TestSmulWWApprox(t *testing.T) {
	// SKP_SMULWW(a,b) ≈ (a*b) >> 16 for small magnitudes.
	for _, a := range []int32{0, 1, -1, 1000, -1000, 1 << 20, -(1 << 20)} {
		for _, b := range []int32{0, 1, -1, 7, -7, 1 << 20, -(1 << 20)} {
			got := SmulWW(a, b)
			want := int32(int64(a) * int64(b) >> 16)
			diff := got - want
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 { // Allow tiny rounding error from the SmulWB+round path.
				t.Errorf("SmulWW(%d,%d)=%d, expected ~%d", a, b, got, want)
			}
		}
	}
}

// Smmul — top 32 bits of a*b.
func TestSmmul(t *testing.T) {
	for _, a := range []int32{0, 1, -1, 1234567, -1234567, int32Max, int32Min + 1} {
		for _, b := range []int32{0, 1, -1, 99999, -99999, int32Max, int32Min + 1} {
			want := int32(int64(a) * int64(b) >> 32)
			if got := Smmul(a, b); got != want {
				t.Errorf("Smmul(%d,%d)=%d want %d", a, b, got, want)
			}
		}
	}
}

func TestClz(t *testing.T) {
	if got := Clz32(0); got != 32 {
		t.Errorf("Clz32(0)=%d", got)
	}
	if got := Clz32(1); got != 31 {
		t.Errorf("Clz32(1)=%d", got)
	}
	if got := Clz32(int32Max); got != 1 {
		t.Errorf("Clz32(max)=%d", got)
	}
	if got := Clz32(-1); got != 0 {
		t.Errorf("Clz32(-1)=%d", got)
	}
	if got := Clz16(0); got != 16 {
		t.Errorf("Clz16(0)=%d", got)
	}
	if got := Clz16(1); got != 15 {
		t.Errorf("Clz16(1)=%d", got)
	}
	if got := Clz64(0); got != 64 {
		t.Errorf("Clz64(0)=%d", got)
	}
}

func TestRor32(t *testing.T) {
	if got := Ror32(0x12345678, 8); got != 0x78123456 {
		t.Errorf("Ror32 right by 8 = %#x", uint32(got))
	}
	if got := Ror32(0x12345678, -8); got != 0x34567812 {
		t.Errorf("Ror32 left by 8 = %#x", uint32(got))
	}
}

func TestNorm32(t *testing.T) {
	if got := Norm32(0); got != 0 {
		t.Errorf("Norm32(0)=%d", got)
	}
	if got := Norm32(int32Min); got != 0 {
		t.Errorf("Norm32(int32Min)=%d", got)
	}
	if got := Norm32(1); got != 30 {
		t.Errorf("Norm32(1)=%d", got)
	}
	if got := Norm32(-2); got != 30 {
		t.Errorf("Norm32(-2)=%d", got)
	}
}

// SqrtApprox — accuracy claim: < 10% above 15, < 2.5% above 120.
func TestSqrtApproxAccuracy(t *testing.T) {
	for _, x := range []int32{16, 100, 1000, 100000, 10_000_000, 1_000_000_000, int32Max} {
		got := float64(SqrtApprox(x))
		want := math.Sqrt(float64(x))
		rel := math.Abs(got-want) / want
		// The C accuracy claim is on the OUTPUT magnitude, not input.
		limit := 0.10
		if want > 120 {
			limit = 0.025
		}
		if rel > limit {
			t.Errorf("SqrtApprox(%d)=%.0f want ~%.0f relErr=%.4f", x, got, want, rel)
		}
	}
	if got := SqrtApprox(0); got != 0 {
		t.Errorf("SqrtApprox(0)=%d", got)
	}
	if got := SqrtApprox(-5); got != 0 {
		t.Errorf("SqrtApprox(-5)=%d", got)
	}
}

func TestSinApproxQ24(t *testing.T) {
	for x := int32(0); x < 65536; x += 137 {
		got := float64(SinApproxQ24(x)) / float64(int64(1)<<24)
		want := math.Sin(float64(x) * 2 * math.Pi / 65536.0)
		if math.Abs(got-want) > 1e-4 {
			t.Errorf("SinApproxQ24(%d)=%.6f want %.6f", x, got, want)
		}
	}
	// Periodicity: 65537 -> 1
	if SinApproxQ24(65537) != SinApproxQ24(1) {
		t.Errorf("SinApproxQ24 not periodic")
	}
	// Cos identity: cos(x) = sin(x + 16384)
	for _, x := range []int32{0, 12345, 32768, 50000} {
		if CosApproxQ24(x) != SinApproxQ24(x+16384) {
			t.Errorf("CosApproxQ24 mismatch at %d", x)
		}
	}
}

func TestDiv32VarQ(t *testing.T) {
	// Spot-check: a/b in Q15.
	cases := []struct {
		a, b int32
		q    int32
	}{
		{1, 1, 15},
		{1234567, 100, 10},
		{-987654, 321, 8},
		{int32Max / 2, 17, 4},
	}
	for _, c := range cases {
		got := Div32VarQ(c.a, c.b, c.q)
		want := float64(c.a) / float64(c.b) * float64(int64(1)<<c.q)
		if math.Abs(float64(got)-want) > math.Abs(want)*1e-3+1 {
			t.Errorf("Div32VarQ(%d,%d,%d)=%d want ~%.0f", c.a, c.b, c.q, got, want)
		}
	}
}

func TestInverse32VarQ(t *testing.T) {
	for _, b := range []int32{1, 2, 3, 100, 12345, -100, -12345, int32Max / 2} {
		for _, q := range []int32{15, 24, 30} {
			got := Inverse32VarQ(b, q)
			want := float64(int64(1)<<q) / float64(b)
			if math.Abs(float64(got)-want) > math.Abs(want)*1e-3+1 {
				t.Errorf("Inverse32VarQ(%d,%d)=%d want ~%.0f", b, q, got, want)
			}
		}
	}
}

func TestRand(t *testing.T) {
	// Seeds should differ for distinct inputs.
	if Rand(0) == Rand(1) {
		t.Errorf("Rand(0)==Rand(1)")
	}
	// Two iterations differ.
	s := int32(42)
	a := Rand(s)
	b := Rand(a)
	if a == b {
		t.Errorf("Rand identity loop")
	}
}

func TestFixConst(t *testing.T) {
	// 0.5 in Q31 = 0x40000000
	if got := FixConst32(0.5, 31); got != 0x40000000 {
		t.Errorf("FixConst32(0.5,31)=%#x", uint32(got))
	}
	// SKP_FIX_CONST adds +0.5 unconditionally; for negative C the bias
	// then truncates toward zero. -0.25*2^30 = -268435456, +0.5 =
	// -268435455.5, (int32) truncates to -268435455. Verified against
	// the C oracle. Earlier expectation -(1<<28) = -268435456 encoded
	// the (incorrect) round-away-from-zero behavior.
	if got := FixConst32(-0.25, 30); got != -268435455 {
		t.Errorf("FixConst32(-0.25,30)=%d", got)
	}
	// FIX_CONST(-2.7, 0) — C produces -2 (round toward +∞ on negatives).
	if got := FixConst32(-2.7, 0); got != -2 {
		t.Errorf("FixConst32(-2.7,0)=%d, want -2", got)
	}
}

func TestLimit(t *testing.T) {
	if got := Limit(50, 0, 100); got != 50 {
		t.Errorf("Limit middle = %d", got)
	}
	if got := Limit(-1, 0, 100); got != 0 {
		t.Errorf("Limit low = %d", got)
	}
	if got := Limit(101, 0, 100); got != 100 {
		t.Errorf("Limit high = %d", got)
	}
	// Reversed bounds (l1 > l2): SKP_LIMIT supports this.
	if got := Limit(50, 100, 0); got != 50 {
		t.Errorf("Limit reversed bounds = %d", got)
	}
}
