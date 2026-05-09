package enc

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dec"
)

// TestEncode40msPacketsRoundTrip — PacketSize=40ms aggregates two frames
// per payload. Each odd-indexed frame should produce 0 bytes (deferred);
// each even-indexed frame closes out the packet. Round-trip through the
// decoder.
func TestEncode40msPacketsRoundTrip(t *testing.T) {
	var encState StateFIX
	InitEncoder(&encState)
	c := EncControl{
		APISampleRate:         16000,
		MaxInternalSampleRate: 16000,
		PacketSize:            640, // 40 ms @ 16 kHz
		BitRate:               25000,
		Complexity:            0,
	}

	const sampleRate = 16000
	in := make([]int16, 320*4) // 80 ms = 2 packets
	for i := range in {
		in[i] = int16(6000 * math.Sin(2*math.Pi*float64(i)/64))
	}

	out := make([]byte, 4096)
	var payload []byte
	deferredCount := 0
	finalizedCount := 0

	for off := 0; off < len(in); off += 320 {
		var n int32 = 1024
		if rc := Encode(&encState, &c, in[off:off+320], 320, out[len(payload):], &n); rc != 0 {
			t.Fatalf("frame at %d: rc=%d", off, rc)
		}
		if n == 0 {
			deferredCount++
		} else {
			finalizedCount++
			payload = append(payload, out[len(payload):len(payload)+int(n)]...)
		}
	}
	if deferredCount != 2 || finalizedCount != 2 {
		t.Errorf("deferred=%d finalized=%d (want 2/2 for 4 frames at 40ms packet)", deferredCount, finalizedCount)
	}

	// Decode the aggregated payload.
	var decState dec.State
	dec.InitDecoder(&decState)
	dctrl := silk.DecControl{APISampleRate: sampleRate}
	pcm := make([]int16, 320*8)
	var samplesOut int32
	rc := dec.Decode(&decState, &dctrl, 0, payload, int32(len(payload)), pcm, &samplesOut)
	if rc != 0 {
		t.Fatalf("Decode rc=%d (payload %d bytes)", rc, len(payload))
	}
	if samplesOut == 0 {
		t.Errorf("decoder produced no samples from multi-frame packet")
	}
}
