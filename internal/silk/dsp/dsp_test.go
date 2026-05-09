package dsp

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// TestInt16ArrayMaxAbs — verify that the result equals max(|x_i|), capped
// at 32767. The C version refuses to return -32768 even for input that hits
// it, since that doesn't fit in int16.
func TestInt16ArrayMaxAbs(t *testing.T) {
	cases := []struct {
		in   []int16
		want int16
	}{
		{[]int16{}, 0},
		{[]int16{0}, 0},
		{[]int16{-100, 50, -200, 75, 32767}, 32767},
		{[]int16{1, 2, 3}, 3},
		{[]int16{-3, -2, -1}, 3},
		{[]int16{-32768, 0, 0}, 32767}, // -32768 squared is (2^15)^2 = 1073741824, > 1073676289 → cap.
		{[]int16{-32767, 0, 0}, 32767},
	}
	for _, c := range cases {
		got := Int16ArrayMaxAbs(c.in, int32(len(c.in)))
		if got != c.want {
			t.Errorf("Int16ArrayMaxAbs(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestScaleCopyVector16(t *testing.T) {
	in := []int16{100, -200, 12345, -1, 0, 32767, -32768}
	out := make([]int16, len(in))
	gainQ16 := int32(0x10000) // 1.0 in Q16
	ScaleCopyVector16(out, in, gainQ16, int32(len(in)))
	// gain = 1.0 → output ≈ input but the SMULWB approximation introduces
	// minor truncation. Check within 1 LSB.
	for i, v := range in {
		diff := int32(out[i]) - int32(v)
		if diff < -1 || diff > 1 {
			t.Errorf("Sample %d: in=%d out=%d", i, v, out[i])
		}
	}

	// gain = 0.5 in Q16 = 0x8000 → output ≈ input/2.
	gainQ16 = 0x8000
	ScaleCopyVector16(out, in, gainQ16, int32(len(in)))
	for i, v := range in {
		want := int32(v) / 2
		diff := int32(out[i]) - want
		if diff < -1 || diff > 1 {
			t.Errorf("0.5x sample %d: in=%d out=%d want~%d", i, v, out[i], want)
		}
	}
}

func TestScaleVector32Q26Lshift18(t *testing.T) {
	data := []int32{1 << 20, -(1 << 20), 100000, -100000}
	gainQ26 := int32(1 << 26) // 1.0 in Q26
	want := make([]int32, len(data))
	for i, v := range data {
		// (v * 2^26) >> 8 = v << 18.
		want[i] = int32(int64(v) * int64(gainQ26) >> 8)
	}
	got := make([]int32, len(data))
	copy(got, data)
	ScaleVector32Q26Lshift18(got, gainQ26, int32(len(got)))
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("idx %d: got %d want %d", i, got[i], want[i])
		}
	}
}

// TestSumSqrShift — checks that the shifted energy * 2^shift approximates
// the true sum of squares within the precision SILK guarantees.
func TestSumSqrShift(t *testing.T) {
	cases := [][]int16{
		{},
		{1, 2, 3},
		{-32768, -32768, -32768, -32768}, // forces shift > 0
		{32767, 32767, 32767, 32767, 32767, 32767, 32767, 32767},
	}
	for _, x := range cases {
		var trueSum int64
		for _, v := range x {
			trueSum += int64(v) * int64(v)
		}
		nrg, shift := SumSqrShift(x, int32(len(x)))
		if nrg < 0 {
			t.Errorf("nrg negative for %v: %d", x, nrg)
			continue
		}
		// Recovered = nrg << shift. Allow tolerance because the C version's
		// precision drops as shift increases (each rescale loses 2 bits).
		recovered := int64(uint32(nrg)) << uint(shift)
		// Tolerance: 4 ULPs of the most-significant scale × 2^shift.
		tol := int64(4) << uint(shift)
		if math.Abs(float64(recovered-trueSum)) > float64(tol) {
			t.Errorf("SumSqrShift(%v): got nrg=%d shift=%d (recovered=%d), want %d", x, nrg, shift, recovered, trueSum)
		}
		// Final result must have at least 2 leading zeros.
		if uint32(nrg)&0xC0000000 != 0 {
			t.Errorf("nrg=%#x has fewer than 2 leading zeros", uint32(nrg))
		}
	}
}

// Random stress: SumSqrShift must not panic and must produce a value with
// at least 2 leading zeros for any int16 input.
func TestSumSqrShiftStress(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 200; trial++ {
		n := rng.Intn(64) + 1
		x := make([]int16, n)
		for i := range x {
			x[i] = int16(rng.Intn(1<<16) - (1 << 15))
		}
		nrg, _ := SumSqrShift(x, int32(n))
		if nrg < 0 {
			t.Errorf("trial %d: nrg<0: %d (%v)", trial, nrg, x)
		}
		if uint32(nrg)&0xC0000000 != 0 {
			t.Errorf("trial %d: too few leading zeros: nrg=%#x", trial, uint32(nrg))
		}
	}
}

func TestInnerProdAligned(t *testing.T) {
	a := []int16{1, 2, 3, -4, 5}
	b := []int16{10, 20, 30, 40, 50}
	want := int32(1*10 + 2*20 + 3*30 + (-4)*40 + 5*50)
	if got := InnerProdAligned(a, b, int32(len(a))); got != want {
		t.Errorf("InnerProdAligned = %d want %d", got, want)
	}
	wantLong := int64(want)
	if got := InnerProd16Aligned64(a, b, int32(len(a))); got != wantLong {
		t.Errorf("InnerProd16Aligned64 = %d want %d", got, wantLong)
	}
}

func TestInsertionSortIncreasing(t *testing.T) {
	a := []int32{5, 3, 8, 1, 9, 2, 7, 4, 6}
	idx := make([]int32, len(a))
	original := make([]int32, len(a))
	copy(original, a)
	K := int32(5)
	InsertionSortIncreasing(a, idx, int32(len(a)), K)
	// First K elements must be sorted increasing and equal to the K smallest
	// values of original.
	sorted := make([]int32, len(original))
	copy(sorted, original)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for i := int32(0); i < K; i++ {
		if a[i] != sorted[i] {
			t.Errorf("sorted prefix [%d] = %d want %d", i, a[i], sorted[i])
		}
		// The index points back to the original position of that value.
		if original[idx[i]] != a[i] {
			t.Errorf("index[%d]=%d points to %d, expected %d", i, idx[i], original[idx[i]], a[i])
		}
	}
}

func TestInsertionSortDecreasingInt16(t *testing.T) {
	a := []int16{1, 7, 3, 9, 2, 8, 4, 6, 5}
	idx := make([]int32, len(a))
	original := make([]int16, len(a))
	copy(original, a)
	K := int32(4)
	InsertionSortDecreasingInt16(a, idx, int32(len(a)), K)
	// First K must be sorted decreasing and equal to the K largest of original.
	sorted := make([]int16, len(original))
	copy(sorted, original)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	for i := int32(0); i < K; i++ {
		if a[i] != sorted[i] {
			t.Errorf("sorted prefix [%d] = %d want %d", i, a[i], sorted[i])
		}
	}
}

func TestInsertionSortIncreasingAllValues(t *testing.T) {
	a := []int32{3, 1, 4, 1, 5, 9, 2, 6, 5, 3, 5}
	want := []int32{1, 1, 2, 3, 3, 4, 5, 5, 5, 6, 9}
	InsertionSortIncreasingAllValues(a, int32(len(a)))
	for i := range a {
		if a[i] != want[i] {
			t.Errorf("a[%d] = %d want %d", i, a[i], want[i])
		}
	}
}

// TestBwExpander — verify chirp behavior:
//   - chirp_Q16 = 1.0 (65536) is identity (after rounding).
//   - chirp_Q16 < 1.0 should reduce magnitudes monotonically (lower-order
//     coefficients shrink less than higher-order ones).
func TestBwExpander(t *testing.T) {
	d := int32(10)
	ar := make([]int16, d)
	for i := range ar {
		ar[i] = 1000
	}
	saved := make([]int16, d)
	copy(saved, ar)

	// chirp = 1.0 → result equals input.
	BwExpander(ar, d, 65536)
	for i := range ar {
		if ar[i] != saved[i] {
			t.Errorf("chirp=1.0 idx %d: got %d, want %d", i, ar[i], saved[i])
		}
	}

	// chirp = 0.9 in Q16 = 58982. Higher-order indices should shrink more.
	copy(ar, saved)
	BwExpander(ar, d, 58982)
	for i := int32(1); i < d; i++ {
		if absInt16(ar[i]) >= absInt16(ar[i-1]) {
			t.Errorf("BwExpander not monotonically shrinking: idx %d=%d, prev=%d", i, ar[i], ar[i-1])
		}
	}
}

func TestBwExpander32(t *testing.T) {
	d := int32(8)
	ar := make([]int32, d)
	for i := range ar {
		ar[i] = 1 << 24
	}
	saved := make([]int32, d)
	copy(saved, ar)

	// chirp = 1.0 → ar[i] ≈ ar[i] (small SmulWW rounding error allowed).
	BwExpander32(ar, d, 65536)
	for i := range ar {
		diff := ar[i] - saved[i]
		if diff < -2 || diff > 2 {
			t.Errorf("chirp=1.0 idx %d: got %d, want ~%d", i, ar[i], saved[i])
		}
	}

	// chirp = 0.9 → monotonic shrink.
	copy(ar, saved)
	BwExpander32(ar, d, 58982)
	for i := int32(1); i < d; i++ {
		if abs32(ar[i]) >= abs32(ar[i-1]) {
			t.Errorf("BwExpander32 not monotonically shrinking: idx %d=%d, prev=%d", i, ar[i], ar[i-1])
		}
	}
}

func absInt16(x int16) int16 {
	if x < 0 {
		return -x
	}
	return x
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
