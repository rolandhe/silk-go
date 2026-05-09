package enc

import (
	"testing"
)

// TestEncodeDTXSuppressesOutput — feeding silence with UseDTX=1 should
// trigger DTX after NoSpeechFramesBeforeDTX frames and zero out the
// payload size.
func TestEncodeDTXSuppressesOutput(t *testing.T) {
	var s StateFIX
	InitEncoder(&s)
	c := EncControl{
		APISampleRate:         16000,
		MaxInternalSampleRate: 16000,
		PacketSize:            320,
		BitRate:               25000,
		Complexity:            0,
		UseDTX:                1,
	}
	in := make([]int16, 320)
	out := make([]byte, 1024)

	dtxStarted := false
	for f := 0; f < 30; f++ {
		var n int32 = 1024
		if rc := Encode(&s, &c, in, 320, out, &n); rc != 0 {
			t.Fatalf("frame %d: rc=%d", f, rc)
		}
		if s.Cmn.InDTX != 0 && n == 0 {
			dtxStarted = true
		}
	}
	if !dtxStarted {
		t.Errorf("DTX never triggered after 30 silent frames")
	}
}
