package enc

import (
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// TestControlAudioBandwidthInitialPick — fresh encoder (FsKHz=0) chooses
// from the target bitrate alone, then clamps to API and max-internal.
func TestControlAudioBandwidthInitialPick(t *testing.T) {
	cases := []struct {
		name          string
		apiHz, maxKHz int32
		bps           int32
		want          int32
	}{
		{"swb-rate-swb-api", 48000, 24, 30000, 24},
		{"swb-rate-wb-api", 16000, 24, 30000, 16},
		{"swb-rate-mb-cap", 48000, 12, 30000, 12},
		{"wb-rate", 48000, 24, 22000, 16},
		{"mb-rate", 48000, 24, 12000, 12},
		{"nb-rate", 48000, 24, 5000, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &CommonState{APIFsHz: tc.apiHz, MaxInternalFsKHz: tc.maxKHz}
			if got := ControlAudioBandwidth(c, tc.bps); got != tc.want {
				t.Errorf("got %d kHz, want %d", got, tc.want)
			}
		})
	}
}

// TestSetupComplexityValues — sanity-check that the three complexity
// presets land on distinct, monotonically increasing knobs (LPC orders,
// MSVQ survivors, delayed decisions).
func TestSetupComplexityValues(t *testing.T) {
	mk := func(c int32) CommonState {
		s := CommonState{FsKHz: 16, PredictLPCOrder: silk.MaxLPCOrder}
		if rc := SetupComplexity(&s, c); rc != 0 {
			t.Fatalf("SetupComplexity(%d) returned %d", c, rc)
		}
		return s
	}
	low := mk(0)
	mid := mk(1)
	hi := mk(2)
	if !(low.PitchEstimationLPCOrder < mid.PitchEstimationLPCOrder &&
		mid.PitchEstimationLPCOrder <= hi.PitchEstimationLPCOrder) {
		t.Errorf("pitch LPC order not increasing: %d %d %d",
			low.PitchEstimationLPCOrder, mid.PitchEstimationLPCOrder, hi.PitchEstimationLPCOrder)
	}
	if !(low.NLSFMSVQSurvivors <= mid.NLSFMSVQSurvivors && mid.NLSFMSVQSurvivors <= hi.NLSFMSVQSurvivors) {
		t.Errorf("MSVQ survivors not non-decreasing")
	}
	if !(low.NStatesDelayedDecision < mid.NStatesDelayedDecision &&
		mid.NStatesDelayedDecision < hi.NStatesDelayedDecision) {
		t.Errorf("delayed-decision states not increasing: %d %d %d",
			low.NStatesDelayedDecision, mid.NStatesDelayedDecision, hi.NStatesDelayedDecision)
	}
	// Out-of-range complexity is rejected.
	var s CommonState
	s.FsKHz = 16
	s.PredictLPCOrder = silk.MaxLPCOrder
	if rc := SetupComplexity(&s, 3); rc != silk.EncInvalidComplexitySetting {
		t.Errorf("complexity=3 → ret %d, want %d", rc, silk.EncInvalidComplexitySetting)
	}
}

// TestControlEncoderFIXFreshState — first invocation populates FsKHz,
// frame length, codebooks, and bitrate thresholds for a 16 kHz target.
func TestControlEncoderFIXFreshState(t *testing.T) {
	var s StateFIX
	if rc := InitEncoderFIX(&s); rc != 0 {
		t.Fatalf("InitEncoderFIX returned %d", rc)
	}
	s.Cmn.APIFsHz = 16000
	s.Cmn.MaxInternalFsKHz = 24

	if rc := ControlEncoderFIX(&s, 20, 0, 0, 1, 22000); rc != 0 {
		t.Fatalf("ControlEncoderFIX returned %d", rc)
	}
	if s.Cmn.FsKHz != 16 {
		t.Errorf("FsKHz = %d, want 16", s.Cmn.FsKHz)
	}
	if s.Cmn.FrameLength != 16*silk.FrameLengthMs {
		t.Errorf("FrameLength = %d, want %d", s.Cmn.FrameLength, 16*silk.FrameLengthMs)
	}
	if s.Cmn.SubfrLength*silk.NBSubFr != s.Cmn.FrameLength {
		t.Errorf("SubfrLength × NBSubFr ≠ FrameLength: %d × %d ≠ %d",
			s.Cmn.SubfrLength, silk.NBSubFr, s.Cmn.FrameLength)
	}
	if s.Cmn.PacketSizeMs != 20 {
		t.Errorf("PacketSizeMs = %d, want 20", s.Cmn.PacketSizeMs)
	}
	if s.Cmn.NLSFCB[0] == nil || s.Cmn.NLSFCB[1] == nil {
		t.Errorf("NLSF codebooks not wired up")
	}
	if s.Cmn.PredictLPCOrder != silk.MaxLPCOrder {
		t.Errorf("PredictLPCOrder = %d, want %d", s.Cmn.PredictLPCOrder, silk.MaxLPCOrder)
	}
	if s.Cmn.ControlledSinceLastPayload != 1 {
		t.Errorf("ControlledSinceLastPayload not latched")
	}
	if s.SNRdBQ7 == 0 {
		t.Errorf("SNR_dB_Q7 not set from rate table")
	}
}

// TestSetupPacketSizeRejectsBad — only the canonical packet sizes are accepted.
func TestSetupPacketSizeRejectsBad(t *testing.T) {
	var s StateFIX
	InitEncoderFIX(&s)
	s.Cmn.APIFsHz = 16000
	s.Cmn.MaxInternalFsKHz = 16
	if rc := setupPacketSizeFIX(&s, 30); rc != silk.EncPacketSizeNotSupported {
		t.Errorf("packet=30 → %d, want %d", rc, silk.EncPacketSizeNotSupported)
	}
	for _, ms := range []int32{20, 40, 60, 80, 100} {
		if rc := setupPacketSizeFIX(&s, ms); rc != 0 {
			t.Errorf("packet=%d → %d, want 0", ms, rc)
		}
	}
}

// TestDetectSWBInputDoesntPanic — exercise the SOS HP filter cascade with
// a few frames of input. We only want to verify it runs end-to-end and
// updates the consecutive-samples counter sensibly.
func TestDetectSWBInputDoesntPanic(t *testing.T) {
	var s DetectSWBState
	in := make([]int16, 320)
	for i := range in {
		// Strong high-frequency content (alternating sign) drives the HP
		// filter output above threshold.
		if i%2 == 0 {
			in[i] = 16000
		} else {
			in[i] = -16000
		}
	}
	for f := 0; f < 5; f++ {
		DetectSWBInput(&s, in, int32(len(in)))
	}
	if s.ConsecSmplsAboveThr <= 0 {
		t.Errorf("ConsecSmplsAboveThr = %d, want > 0 for HP-rich input", s.ConsecSmplsAboveThr)
	}
}

// TestLBRRReset — every slot's Usage is reset to NoLBRR.
func TestLBRRReset(t *testing.T) {
	var c CommonState
	for i := range c.LBRRBuffer {
		c.LBRRBuffer[i].Usage = 99
	}
	LBRRReset(&c)
	for i, b := range c.LBRRBuffer {
		if b.Usage != silk.NoLBRR {
			t.Errorf("slot %d Usage = %d, want %d", i, b.Usage, silk.NoLBRR)
		}
	}
}
