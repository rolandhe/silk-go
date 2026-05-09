package dec

import (
	"math/rand"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
)

// TestDecodePulsesAllZero — encode an all-zero pulse vector; decoder must
// recover all zeros.
func TestDecodePulsesAllZero(t *testing.T) {
	const N = 80 // 5 shell blocks of 16
	q := make([]int8, N)

	enc := &rangecoder.State{}
	enc.EncInit()
	EncodePulses(enc, 0 /*sigtype*/, 0 /*QoffType*/, q, N)
	enc.EncWrapUp()
	_, nBytes := enc.GetLength()

	dec := &rangecoder.State{}
	dec.DecInit(enc.Buffer[:nBytes])

	ctrl := &Control{Sigtype: 0, QuantOffsetType: 0}
	out := make([]int32, N)
	DecodePulses(dec, ctrl, out, N)

	for i, v := range out {
		if v != 0 {
			t.Errorf("idx %d: got %d, want 0", i, v)
		}
	}
}

// TestDecodePulsesSparse — sparse small-magnitude pulses; encode/decode
// round-trip should reproduce them bit-for-bit, including signs.
func TestDecodePulsesSparse(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for trial := 0; trial < 20; trial++ {
		const N = 80
		q := make([]int8, N)
		for i := 0; i < N; i++ {
			// Mostly zero, occasional small ±value.
			if rng.Intn(4) == 0 {
				v := int8(rng.Intn(3) + 1)
				if rng.Intn(2) == 0 {
					v = -v
				}
				q[i] = v
			}
		}
		// Total per shell block must stay within max_pulses_table[3] envelope.
		// Trim if a shell block accumulates too many pulses.
		for blk := int32(0); blk < N/silk.ShellCodecFrameLength; blk++ {
			off := blk * silk.ShellCodecFrameLength
			sum := 0
			for k := int32(0); k < silk.ShellCodecFrameLength; k++ {
				v := int32(q[off+k])
				if v < 0 {
					v = -v
				}
				sum += int(v)
			}
			// Cap at MaxPulses so sumPulses won't blow past the table.
			for k := int32(0); k < silk.ShellCodecFrameLength && sum > silk.MaxPulses; k++ {
				if q[off+k] != 0 {
					q[off+k] = 0
					sum--
				}
			}
		}

		sigtype := int32(rng.Intn(2))
		qoff := int32(rng.Intn(2))

		enc := &rangecoder.State{}
		enc.EncInit()
		EncodePulses(enc, sigtype, qoff, q, N)
		enc.EncWrapUp()
		_, nBytes := enc.GetLength()

		dec := &rangecoder.State{}
		dec.DecInit(enc.Buffer[:nBytes])
		ctrl := &Control{Sigtype: sigtype, QuantOffsetType: qoff}
		out := make([]int32, N)
		DecodePulses(dec, ctrl, out, N)

		for i := 0; i < N; i++ {
			if int32(q[i]) != out[i] {
				t.Errorf("trial %d idx %d: in=%d out=%d", trial, i, q[i], out[i])
			}
		}
	}
}

// TestDecodePulsesLargeMagnitudes — pulses big enough to trigger the
// nRshifts (LSB-extension) path. round-trip must still recover the
// originals.
func TestDecodePulsesLSBExtension(t *testing.T) {
	const N = 32 // 2 shell blocks
	q := make([]int8, N)
	// One block with several large pulses to force at least 1 LSB shift.
	q[0] = 30
	q[1] = -25
	q[2] = 40
	q[3] = -20

	enc := &rangecoder.State{}
	enc.EncInit()
	EncodePulses(enc, 0, 0, q, N)
	enc.EncWrapUp()
	_, nBytes := enc.GetLength()

	dec := &rangecoder.State{}
	dec.DecInit(enc.Buffer[:nBytes])
	ctrl := &Control{Sigtype: 0, QuantOffsetType: 0}
	out := make([]int32, N)
	DecodePulses(dec, ctrl, out, N)

	for i := 0; i < N; i++ {
		if int32(q[i]) != out[i] {
			t.Errorf("idx %d: in=%d out=%d", i, q[i], out[i])
		}
	}
}
