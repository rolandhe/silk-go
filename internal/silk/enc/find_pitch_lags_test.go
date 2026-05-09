package enc

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// TestFindPitchLagsFIXVoiced — feed a periodic signal through the full
// pitch pipeline (LPC residual + multi-stage search) and verify it lands
// on a voiced verdict with sensible per-subframe lags.
func TestFindPitchLagsFIXVoiced(t *testing.T) {
	var s StateFIX
	if rc := InitEncoderFIX(&s); rc != 0 {
		t.Fatalf("InitEncoderFIX returned %d", rc)
	}
	s.Cmn.APIFsHz = 16000
	s.Cmn.MaxInternalFsKHz = 16
	if rc := ControlEncoderFIX(&s, 20, 0, 0, 1, 22000); rc != 0 {
		t.Fatalf("ControlEncoderFIX returned %d", rc)
	}
	s.SpeechActivityQ8 = 200

	// Fill XBuf with a periodic signal occupying the full pitch-analysis
	// window (frame_length back-history + frame_length current + LA_pitch
	// look-ahead).
	const periodSamples = 64 // 250 Hz at 16 kHz
	frameLen := s.Cmn.FrameLength
	winSamples := frameLen + frameLen + s.Cmn.LAPitch
	for i := int32(0); i < winSamples; i++ {
		s.XBuf[i] = int16(6000 * math.Sin(2*math.Pi*float64(i)/periodSamples))
	}

	// Pre-compute the `res` buffer length the C source uses: la_pitch +
	// 2*frame_length samples.
	resLen := s.Cmn.LAPitch + 2*frameLen
	res := make([]int16, resLen)

	var ctrl ControlFIX
	ctrl.InputTiltQ15 = 0
	FindPitchLagsFIX(&s, &ctrl, res, s.XBuf[:], frameLen)

	if ctrl.Cmn.Sigtype != silk.SigTypeVoiced {
		t.Fatalf("Sigtype = %d, want voiced (%d)", ctrl.Cmn.Sigtype, silk.SigTypeVoiced)
	}
	for i, p := range ctrl.Cmn.PitchL {
		if p < periodSamples/4 || p > periodSamples*4 {
			t.Errorf("subfr %d pitch %d not near %d", i, p, periodSamples)
		}
	}
	if s.LTPCorrQ15 <= 0 {
		t.Errorf("LTPCorrQ15 = %d, want > 0", s.LTPCorrQ15)
	}
	if ctrl.PredGainQ16 <= 0 {
		t.Errorf("PredGainQ16 = %d, want > 0", ctrl.PredGainQ16)
	}
}

// TestFindPitchLagsFIXSilenceUnvoiced — silence → unvoiced verdict + zero
// pitch.
func TestFindPitchLagsFIXSilenceUnvoiced(t *testing.T) {
	var s StateFIX
	InitEncoderFIX(&s)
	s.Cmn.APIFsHz = 16000
	s.Cmn.MaxInternalFsKHz = 16
	ControlEncoderFIX(&s, 20, 0, 0, 1, 22000)
	s.SpeechActivityQ8 = 50 // low activity

	resLen := s.Cmn.LAPitch + 2*s.Cmn.FrameLength
	res := make([]int16, resLen)

	var ctrl ControlFIX
	FindPitchLagsFIX(&s, &ctrl, res, s.XBuf[:], s.Cmn.FrameLength)

	if ctrl.Cmn.Sigtype != silk.SigTypeUnvoiced {
		t.Errorf("silence Sigtype = %d, want unvoiced", ctrl.Cmn.Sigtype)
	}
	for i, p := range ctrl.Cmn.PitchL {
		if p != 0 {
			t.Errorf("silence subfr %d pitch %d, want 0", i, p)
		}
	}
}
