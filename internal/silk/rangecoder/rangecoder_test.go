package rangecoder

import (
	"math/rand"
	"testing"
)

// makeUniformCDF builds a CDF of N equally-likely symbols, scaled to 0..0xFFFF.
func makeUniformCDF(n int) []uint16 {
	cdf := make([]uint16, n+1)
	for i := 0; i <= n; i++ {
		cdf[i] = uint16(uint32(i) * 0xFFFF / uint32(n))
	}
	cdf[n] = 0xFFFF
	return cdf
}

// TestRoundTripUniform — encode a sequence of symbols against a uniform CDF
// and verify the decoder reproduces them exactly.
func TestRoundTripUniform(t *testing.T) {
	cdf := makeUniformCDF(8)
	probIx := int32(4) // middle entry

	rng := rand.New(rand.NewSource(1))
	symbols := make([]int32, 200)
	for i := range symbols {
		symbols[i] = int32(rng.Intn(8))
	}

	enc := &State{}
	enc.EncInit()
	for _, s := range symbols {
		enc.Encode(s, cdf)
	}
	if enc.Error != 0 {
		t.Fatalf("encoder error: %d", enc.Error)
	}
	enc.EncWrapUp()
	_, nBytes := enc.GetLength()

	dec := &State{}
	dec.DecInit(enc.Buffer[:nBytes])
	for i, want := range symbols {
		got := dec.Decode(cdf, probIx)
		if dec.Error != 0 {
			t.Fatalf("decoder error at %d: %d", i, dec.Error)
		}
		if got != want {
			t.Fatalf("symbol %d: got %d want %d", i, got, want)
		}
	}
	dec.CheckAfterDecoding()
	if dec.Error != 0 {
		t.Fatalf("CheckAfterDecoding: %d", dec.Error)
	}
}

// TestRoundTripSkewedCDF — same idea but with a skewed CDF that concentrates
// probability mass on a few symbols. Tests carry propagation through the
// encoder more thoroughly.
func TestRoundTripSkewedCDF(t *testing.T) {
	// 4-symbol CDF with masses ~ {0.05, 0.05, 0.05, 0.85}
	cdf := []uint16{0, 3276, 6553, 9830, 0xFFFF}
	probIx := int32(2)

	rng := rand.New(rand.NewSource(7))
	symbols := make([]int32, 500)
	for i := range symbols {
		r := rng.Intn(100)
		switch {
		case r < 5:
			symbols[i] = 0
		case r < 10:
			symbols[i] = 1
		case r < 15:
			symbols[i] = 2
		default:
			symbols[i] = 3
		}
	}

	enc := &State{}
	enc.EncInit()
	for _, s := range symbols {
		enc.Encode(s, cdf)
		if enc.Error != 0 {
			t.Fatalf("encode error after %d symbols: %d", 0, enc.Error)
		}
	}
	enc.EncWrapUp()
	_, nBytes := enc.GetLength()

	dec := &State{}
	dec.DecInit(enc.Buffer[:nBytes])
	for i, want := range symbols {
		got := dec.Decode(cdf, probIx)
		if dec.Error != 0 {
			t.Fatalf("decode error at %d: %d", i, dec.Error)
		}
		if got != want {
			t.Fatalf("symbol %d: got %d want %d", i, got, want)
		}
	}
	dec.CheckAfterDecoding()
	if dec.Error != 0 {
		t.Fatalf("CheckAfterDecoding: %d", dec.Error)
	}
}

// TestEncodeWithMultipleCDFs — different CDF per symbol position.
func TestRoundTripMultipleCDFs(t *testing.T) {
	cdfs := [][]uint16{
		makeUniformCDF(4),
		makeUniformCDF(8),
		makeUniformCDF(16),
		makeUniformCDF(2),
	}
	startIx := []int32{2, 4, 8, 1}

	rng := rand.New(rand.NewSource(42))
	const N = 30
	symbols := make([]int32, N*len(cdfs))
	probsPerSym := make([][]uint16, N*len(cdfs))
	startIxes := make([]int32, N*len(cdfs))
	encOrder := make([]int, N*len(cdfs))
	for i := 0; i < N; i++ {
		for j, cdf := range cdfs {
			idx := i*len(cdfs) + j
			max := len(cdf) - 1
			symbols[idx] = int32(rng.Intn(max))
			probsPerSym[idx] = cdf
			startIxes[idx] = startIx[j]
			encOrder[idx] = idx
		}
	}

	enc := &State{}
	enc.EncInit()
	enc.EncodeMulti(symbols, probsPerSym)
	if enc.Error != 0 {
		t.Fatalf("encode error: %d", enc.Error)
	}
	enc.EncWrapUp()
	_, nBytes := enc.GetLength()

	dec := &State{}
	dec.DecInit(enc.Buffer[:nBytes])
	got := dec.DecodeMulti(probsPerSym, startIxes)
	if dec.Error != 0 {
		t.Fatalf("decode error: %d", dec.Error)
	}
	for i := range symbols {
		if got[i] != symbols[i] {
			t.Fatalf("symbol %d: got %d want %d", i, got[i], symbols[i])
		}
	}
}

func TestEmptyInit(t *testing.T) {
	enc := &State{}
	enc.EncInit()
	if enc.RangeQ16 != 0xFFFF {
		t.Errorf("RangeQ16 = %x, want FFFF", enc.RangeQ16)
	}
	if enc.BufferLength != 1024 {
		t.Errorf("BufferLength = %d", enc.BufferLength)
	}
}
