package enc

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dec"
)

// TestEncodeAtAllComplexities — round-trip at complexity 0/1/2. Higher
// complexity exercises NSQDelDec + warped LPC analysis.
func TestEncodeAtAllComplexities(t *testing.T) {
	for _, complexity := range []int32{0, 1, 2} {
		t.Run("", func(t *testing.T) {
			var encState StateFIX
			InitEncoder(&encState)
			c := EncControl{
				APISampleRate:         16000,
				MaxInternalSampleRate: 16000,
				PacketSize:            320,
				BitRate:               25000,
				Complexity:            complexity,
			}
			const sampleRate = 16000
			in := make([]int16, 320*3) // 60 ms
			for i := range in {
				in[i] = int16(6000*math.Sin(2*math.Pi*float64(i)/64) +
					2000*math.Sin(2*math.Pi*float64(i)/40))
			}

			out := make([]byte, 4096)
			var payload []byte
			for off := 0; off < len(in); off += 320 {
				var n int32 = 1024
				if rc := Encode(&encState, &c, in[off:off+320], 320, out[len(payload):], &n); rc != 0 {
					t.Fatalf("complexity=%d frame %d rc=%d", complexity, off, rc)
				}
				if n > 0 {
					payload = append(payload, out[len(payload):len(payload)+int(n)]...)
				}
			}
			if len(payload) == 0 {
				t.Fatalf("complexity=%d: no payload produced", complexity)
			}

			var decState dec.State
			dec.InitDecoder(&decState)
			dctrl := silk.DecControl{APISampleRate: sampleRate}
			pcm := make([]int16, 320*5)
			var samplesOut int32
			rc := dec.Decode(&decState, &dctrl, 0, payload, int32(len(payload)), pcm, &samplesOut)
			if rc != 0 {
				t.Fatalf("complexity=%d Decode rc=%d", complexity, rc)
			}
			if samplesOut == 0 {
				t.Errorf("complexity=%d: 0 decoded samples", complexity)
			}
		})
	}
}
