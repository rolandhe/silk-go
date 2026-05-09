package enc

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dec"
)

// TestEncodeWithFEC — UseInBandFEC=1 + lossy conditions should produce a
// payload that the decoder still accepts. Doesn't validate the FEC
// recovery logic (no actual loss simulated), just that the encoder
// doesn't choke on the FEC code path.
func TestEncodeWithFEC(t *testing.T) {
	var encState StateFIX
	InitEncoder(&encState)
	c := EncControl{
		APISampleRate:         16000,
		MaxInternalSampleRate: 16000,
		PacketSize:            320,
		BitRate:               25000,
		Complexity:            0,
		UseInBandFEC:          1,
		PacketLossPercentage:  20,
	}
	const sampleRate = 16000
	in := make([]int16, 320*5) // 100 ms
	for i := range in {
		in[i] = int16(7000 * math.Sin(2*math.Pi*float64(i)/64))
	}

	out := make([]byte, 4096)
	var payload []byte
	for off := 0; off < len(in); off += 320 {
		var n int32 = 1024
		if rc := Encode(&encState, &c, in[off:off+320], 320, out[len(payload):], &n); rc != 0 {
			t.Fatalf("frame %d: rc=%d", off, rc)
		}
		if n > 0 {
			payload = append(payload, out[len(payload):len(payload)+int(n)]...)
		}
	}
	if len(payload) == 0 {
		t.Fatal("FEC encoder emitted no payload")
	}

	// Decode.
	var decState dec.State
	dec.InitDecoder(&decState)
	dctrl := silk.DecControl{APISampleRate: sampleRate}
	pcm := make([]int16, 320*10)
	var samplesOut int32
	rc := dec.Decode(&decState, &dctrl, 0, payload, int32(len(payload)), pcm, &samplesOut)
	if rc != 0 {
		t.Fatalf("Decode rc=%d (payload %d bytes)", rc, len(payload))
	}
	if samplesOut == 0 {
		t.Errorf("decoder produced no samples from FEC-enabled stream")
	}
}
