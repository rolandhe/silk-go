package enc

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// makeEncoderForShape — initialize a StateFIX configured for 16 kHz with
// a fresh control struct prepped enough for the noise-shape analysis to
// run end-to-end. Returns the encoder, the control struct, the input
// buffer (with la_shape preceding samples + frame_length + la_shape
// look-ahead), the offset into that buffer where the current frame
// starts, and the LPC residual.
func makeEncoderForShape(t *testing.T, fsKHz, complexity int32) (*StateFIX, *ControlFIX, []int16, int32, []int16) {
	t.Helper()

	var s StateFIX
	if rc := InitEncoderFIX(&s); rc != 0 {
		t.Fatalf("InitEncoderFIX returned %d", rc)
	}
	s.Cmn.APIFsHz = fsKHz * 1000
	s.Cmn.MaxInternalFsKHz = fsKHz
	if rc := ControlEncoderFIX(&s, 20, 0, 0, complexity, 22000); rc != 0 {
		t.Fatalf("ControlEncoderFIX returned %d", rc)
	}
	s.SpeechActivityQ8 = 200
	s.LTPCorrQ15 = 16384

	var ctrl ControlFIX
	for i := range ctrl.GainsQ16 {
		ctrl.GainsQ16[i] = 1 << 18
	}
	for i := range ctrl.Cmn.PitchL {
		ctrl.Cmn.PitchL[i] = 80 // ~200 Hz at 16 kHz
	}
	ctrl.Cmn.Sigtype = silk.SigTypeVoiced
	ctrl.PredGainQ16 = 1 << 16
	for i := range ctrl.InputQualityBandsQ15 {
		ctrl.InputQualityBandsQ15[i] = 1 << 14 // mid quality
	}

	// xBuf has la_shape preceding samples + frame_length current +
	// la_shape look-ahead. The current frame begins at offset la_shape.
	la := s.Cmn.LAShape
	winLen := s.Cmn.FrameLength + 2*la
	xBuf := make([]int16, winLen)
	for i := int32(0); i < winLen; i++ {
		xBuf[i] = int16(4000 * math.Sin(2*math.Pi*float64(i)/40))
	}
	xCurrentOff := la

	// pitch residual buffer — used only to compute sparseness on unvoiced;
	// for voiced its content doesn't matter but it must be at least
	// frame_length long.
	res := make([]int16, s.Cmn.FrameLength)
	for i := range res {
		res[i] = int16(1000 * math.Sin(2*math.Pi*float64(i)/40))
	}
	return &s, &ctrl, xBuf, xCurrentOff, res
}

// TestNoiseShapeAnalysisSmokeVoiced — voiced signal, mid complexity.
// Verifies the routine runs end-to-end and populates per-subframe gain,
// AR1/AR2, LF shape, tilt and harm-boost outputs sensibly.
func TestNoiseShapeAnalysisSmokeVoiced(t *testing.T) {
	s, ctrl, xBuf, xOff, res := makeEncoderForShape(t, 16, 1)
	NoiseShapeAnalysisFIX(s, ctrl, res, xBuf, xOff)

	for k := 0; k < silk.NBSubFr; k++ {
		if ctrl.GainsQ16[k] <= 0 {
			t.Errorf("subfr %d: GainsQ16 = %d", k, ctrl.GainsQ16[k])
		}
		if ctrl.GainsPreQ14[k] <= 0 {
			t.Errorf("subfr %d: GainsPreQ14 = %d", k, ctrl.GainsPreQ14[k])
		}
		// At least one AR1 coefficient should be non-zero in normal speech.
		nonzero := false
		for i := int32(0); i < s.Cmn.ShapingLPCOrder; i++ {
			if ctrl.AR1Q13[int32(k)*silk.MaxShapeLPCOrder+i] != 0 {
				nonzero = true
				break
			}
		}
		if !nonzero {
			t.Errorf("subfr %d: AR1Q13 row all zero", k)
		}
	}
	if ctrl.CurrentSNRdBQ7 == 0 {
		t.Errorf("CurrentSNRdBQ7 not set")
	}
	if ctrl.InputQualityQ14 == 0 {
		t.Errorf("InputQualityQ14 not set")
	}
}

// TestNoiseShapeAnalysisUnvoicedSparseness — unvoiced input must populate
// SparsenessQ8 and pick a quantizer offset accordingly.
func TestNoiseShapeAnalysisUnvoicedSparseness(t *testing.T) {
	s, ctrl, xBuf, xOff, res := makeEncoderForShape(t, 16, 1)
	ctrl.Cmn.Sigtype = silk.SigTypeUnvoiced

	NoiseShapeAnalysisFIX(s, ctrl, res, xBuf, xOff)

	if ctrl.SparsenessQ8 < 0 || ctrl.SparsenessQ8 > 256 {
		t.Errorf("SparsenessQ8 = %d, want in [0, 256]", ctrl.SparsenessQ8)
	}
	if ctrl.Cmn.QuantOffsetType != 0 && ctrl.Cmn.QuantOffsetType != 1 {
		t.Errorf("QuantOffsetType = %d", ctrl.Cmn.QuantOffsetType)
	}
}

// TestPrefilterFIXSmoke — prefilter the noise-shape output and verify it
// produces non-zero xw[] without panic.
func TestPrefilterFIXSmoke(t *testing.T) {
	s, ctrl, xBuf, xOff, res := makeEncoderForShape(t, 16, 1)
	NoiseShapeAnalysisFIX(s, ctrl, res, xBuf, xOff)

	xw := make([]int16, s.Cmn.FrameLength)
	PrefilterFIX(s, ctrl, xw, xBuf[xOff:])

	// At least one sample should be non-zero given the periodic input.
	nonzero := false
	for _, v := range xw {
		if v != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Errorf("prefilter output is all zero")
	}
	if s.SPrefilt.LagPrev != ctrl.Cmn.PitchL[silk.NBSubFr-1] {
		t.Errorf("LagPrev = %d, want %d", s.SPrefilt.LagPrev, ctrl.Cmn.PitchL[silk.NBSubFr-1])
	}
}
