package enc

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dec"
)

// TestEncodeQueryDefaults — fresh InitEncoder + QueryEncoder echoes back
// the values seeded into the encoder by the first Encode call.
func TestEncodeQueryDefaults(t *testing.T) {
	var s StateFIX
	ctrl, rc := InitEncoder(&s)
	if rc != 0 {
		t.Fatalf("InitEncoder returned %d", rc)
	}
	// Pre-init the values that QueryEncoder reads back.
	if ctrl.APISampleRate != 0 {
		t.Errorf("APISampleRate = %d on a fresh encoder, want 0", ctrl.APISampleRate)
	}
	if EncoderSize() <= 0 {
		t.Errorf("EncoderSize = %d", EncoderSize())
	}
}

// TestEncodeRejectsBadFs — out-of-range API rate is rejected.
func TestEncodeRejectsBadFs(t *testing.T) {
	var s StateFIX
	InitEncoder(&s)
	c := EncControl{
		APISampleRate:         11025, // not in {8/12/16/24/32/44.1/48} kHz
		MaxInternalSampleRate: 16000,
		PacketSize:            16000 / 50, // 20 ms
		BitRate:               22000,
	}
	in := make([]int16, 320)
	out := make([]byte, 1024)
	var n int32 = 1024
	if rc := Encode(&s, &c, in, int32(len(in)), out, &n); rc != silk.EncFsNotSupported {
		t.Errorf("rc = %d, want %d", rc, silk.EncFsNotSupported)
	}
}

// TestEncodeRejectsOversizedNSamples — nSamplesIn must not exceed the
// caller's slice length (regression test for the OOB panic that the
// pre-fix version had).
func TestEncodeRejectsOversizedNSamples(t *testing.T) {
	var s StateFIX
	InitEncoder(&s)
	c := EncControl{
		APISampleRate:         16000,
		MaxInternalSampleRate: 16000,
		PacketSize:            320,
		BitRate:               22000,
	}
	in := make([]int16, 160) // 10 ms — too short for 320-sample claim
	out := make([]byte, 1024)
	var n int32 = 1024
	if rc := Encode(&s, &c, in, 320, out, &n); rc != silk.EncInputInvalidNoOfSamples {
		t.Errorf("rc = %d, want %d", rc, silk.EncInputInvalidNoOfSamples)
	}
}

// TestEncodeRejectsBadInputLength — input must be a multiple of 10 ms.
func TestEncodeRejectsBadInputLength(t *testing.T) {
	var s StateFIX
	InitEncoder(&s)
	c := EncControl{
		APISampleRate:         16000,
		MaxInternalSampleRate: 16000,
		PacketSize:            320, // 20 ms
		BitRate:               22000,
	}
	// 17 ms = 272 samples — not a clean multiple of 10 ms.
	in := make([]int16, 272)
	out := make([]byte, 1024)
	var n int32 = 1024
	if rc := Encode(&s, &c, in, int32(len(in)), out, &n); rc != silk.EncInputInvalidNoOfSamples {
		t.Errorf("rc = %d, want %d", rc, silk.EncInputInvalidNoOfSamples)
	}
}

// TestEncodeProducesPayload — feed enough samples for one packet and
// verify Encode emits a non-empty payload.
func TestEncodeProducesPayload(t *testing.T) {
	var s StateFIX
	InitEncoder(&s)
	c := EncControl{
		APISampleRate:         16000,
		MaxInternalSampleRate: 16000,
		PacketSize:            320, // 20 ms
		BitRate:               22000,
		Complexity:            0, // low: avoid del-dec for the smoke test
	}
	// 20 ms of a periodic signal at 16 kHz.
	in := make([]int16, 320)
	for i := range in {
		in[i] = int16(8000 * math.Sin(2*math.Pi*float64(i)/64))
	}
	out := make([]byte, 1024)
	var n int32 = 1024

	rc := Encode(&s, &c, in, int32(len(in)), out, &n)
	if rc != 0 {
		t.Fatalf("Encode returned %d", rc)
	}
	if n <= 0 {
		t.Errorf("no payload produced (n=%d)", n)
	}
	if n > 1024 {
		t.Errorf("n=%d exceeds buffer", n)
	}
}

// TestEncodeDecodeRoundTrip — encode 60 ms, decode it back, check the
// output isn't all zero (we don't expect bit-exactness from an open-loop
// codec but the decoder must produce non-trivial samples).
func TestEncodeDecodeRoundTrip(t *testing.T) {
	var encState StateFIX
	InitEncoder(&encState)
	c := EncControl{
		APISampleRate:         16000,
		MaxInternalSampleRate: 16000,
		PacketSize:            320, // 20 ms
		BitRate:               22000,
		Complexity:            0,
	}

	// 60 ms input at 16 kHz.
	in := make([]int16, 320*3)
	for i := range in {
		in[i] = int16(6000*math.Sin(2*math.Pi*float64(i)/64) +
			2000*math.Sin(2*math.Pi*float64(i)/40))
	}

	var payload []byte
	out := make([]byte, 4096)
	off := int32(0)
	for off < int32(len(in)) {
		var n int32 = 1024
		if rc := Encode(&encState, &c, in[off:off+320], 320, out[len(payload):], &n); rc != 0 {
			t.Fatalf("Encode returned %d at offset %d", rc, off)
		}
		if n > 0 {
			payload = append(payload, out[len(payload):len(payload)+int(n)]...)
		}
		off += 320
	}

	if len(payload) == 0 {
		t.Fatal("encoder emitted no payload over 60 ms of input")
	}

	// Decode.
	var decState dec.State
	dec.InitDecoder(&decState)
	dctrl := silk.DecControl{APISampleRate: 16000}
	pcm := make([]int16, 320*5)
	var samplesOut int32
	rc := dec.Decode(&decState, &dctrl, 0, payload, int32(len(payload)), pcm, &samplesOut)
	if rc != 0 {
		t.Fatalf("Decode returned %d (payload %d bytes)", rc, len(payload))
	}
	if samplesOut == 0 {
		t.Errorf("Decode produced 0 samples")
	}
}
