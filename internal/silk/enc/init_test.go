package enc

import "testing"

// TestInitEncoderFIXDefaults — InitEncoderFIX zeros the state, primes the
// HP smoothers, and arms the post-reset flag and NSQ gain.
func TestInitEncoderFIXDefaults(t *testing.T) {
	var s StateFIX
	// Dirty values should all be wiped.
	s.Cmn.PrevSigtype = 99
	s.Cmn.SNSQ.PrevInvGainQ16 = 12345
	s.VariableHPSmth1Q15 = -1

	if rc := InitEncoderFIX(&s); rc != 0 {
		t.Fatalf("InitEncoderFIX returned %d", rc)
	}
	if s.VariableHPSmth1Q15 != 200844 || s.VariableHPSmth2Q15 != 200844 {
		t.Errorf("HP smoothers: %d %d", s.VariableHPSmth1Q15, s.VariableHPSmth2Q15)
	}
	if s.Cmn.FirstFrameAfterReset != 1 {
		t.Errorf("FirstFrameAfterReset = %d", s.Cmn.FirstFrameAfterReset)
	}
	if s.Cmn.SNSQ.PrevInvGainQ16 != 65536 || s.Cmn.SNSQLBRR.PrevInvGainQ16 != 65536 {
		t.Errorf("NSQ PrevInvGainQ16: %d %d", s.Cmn.SNSQ.PrevInvGainQ16, s.Cmn.SNSQLBRR.PrevInvGainQ16)
	}
	if s.Cmn.PrevSigtype != 0 {
		t.Errorf("PrevSigtype not zeroed: %d", s.Cmn.PrevSigtype)
	}
	// VAD seeded its counter to 15 in Init.
	if s.Cmn.SVAD.Counter != 15 {
		t.Errorf("VAD counter = %d, want 15", s.Cmn.SVAD.Counter)
	}
}

// TestLPVariableCutoffPassthrough — TransitionFrameNo == 0 must copy input
// straight to output (no biquad applied).
func TestLPVariableCutoffPassthrough(t *testing.T) {
	var lp LPState
	in := []int16{100, -200, 300, -400, 500, -600}
	out := make([]int16, len(in))
	LPVariableCutoff(&lp, out, in, int32(len(in)))
	for i, v := range in {
		if out[i] != v {
			t.Errorf("idx %d: got %d want %d", i, out[i], v)
		}
	}
}

// TestLPVariableCutoffRunsTransition — exercise the down-ramp path so we
// catch any panics in the table-interpolation switch (we don't validate
// audio quality, just that it runs and produces non-zero output).
func TestLPVariableCutoffRunsTransition(t *testing.T) {
	var lp LPState
	lp.TransitionFrameNo = 1 // start a down-ramp
	lp.Mode = 0
	const n = 320
	in := make([]int16, n)
	for i := range in {
		in[i] = int16(1000 * (i % 7))
	}
	out := make([]int16, n)
	LPVariableCutoff(&lp, out, in, n)
	// Frame counter must advance for active down-ramp.
	if lp.TransitionFrameNo != 2 {
		t.Errorf("TransitionFrameNo = %d, want 2", lp.TransitionFrameNo)
	}
}
