package dec

import (
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// setupCoreState configures a minimal but valid State for DecodeCore at a
// given fs (8 or 16 kHz) with the given sigtype.
func setupCoreState(fsKHz int32, sigtype int32) (*State, *Control) {
	s := &State{}
	DecoderSetFs(s, fsKHz)
	s.PrevInvGainQ16 = 1 << 15

	c := &Control{
		Sigtype:          sigtype,
		QuantOffsetType:  0,
		Seed:             1,
		NLSFInterpCoefQ2: 4,
	}
	for i := range c.GainsQ16 {
		c.GainsQ16[i] = 1 << 18 // 4.0 in Q16
	}
	for i := range c.PitchL {
		c.PitchL[i] = 50
	}
	// Gentle mid-band LPC: a₁ = 0.5 in Q12, others 0.
	for half := 0; half < 2; half++ {
		for i := int32(0); i < silk.MaxLPCOrder; i++ {
			c.PredCoefQ12[half][i] = 0
		}
		c.PredCoefQ12[half][0] = 1 << 11 // 0.5 in Q12
	}
	if sigtype == silk.SigTypeVoiced {
		// Center LTP coefficient = 0.25 in Q14.
		for k := 0; k < silk.NBSubFr; k++ {
			c.LTPCoefQ14[k*silk.LTPOrder+silk.LTPOrder/2] = 1 << 12
		}
		c.LTPScaleQ14 = 1 << 14 // 1.0
	}
	return s, c
}

// TestDecodeCoreUnvoicedZeroPulses — feed all-zero pulses, unvoiced.
// Output should be deterministic and bounded (no NaN/explode/saturation
// across the whole frame).
func TestDecodeCoreUnvoicedZeroPulses(t *testing.T) {
	s, c := setupCoreState(8, silk.SigTypeUnvoiced)
	q := make([]int32, s.FrameLength)
	xq := make([]int16, s.FrameLength)
	DecodeCore(s, c, xq, q)

	// Output should be in int16 range (Sat16 enforced) — verify no all-clipped output.
	clipped := 0
	for _, v := range xq {
		if v == 32767 || v == -32768 {
			clipped++
		}
	}
	if clipped > 5 {
		t.Errorf("too many clipped samples (%d) for zero pulses", clipped)
	}
}

// TestDecodeCoreVoicedNonZeroOutputs — voiced frame with a few pulses
// should produce non-zero output.
func TestDecodeCoreVoicedNonZeroOutputs(t *testing.T) {
	s, c := setupCoreState(16, silk.SigTypeVoiced)
	q := make([]int32, s.FrameLength)
	for i := int32(0); i < s.FrameLength; i += 16 {
		q[i] = 3
	}
	xq := make([]int16, s.FrameLength)
	DecodeCore(s, c, xq, q)

	nonzero := 0
	for _, v := range xq {
		if v != 0 {
			nonzero++
		}
	}
	if nonzero < int(s.FrameLength)/4 {
		t.Errorf("only %d/%d nonzero samples", nonzero, s.FrameLength)
	}
}

// TestDecodeCoreDeterminism — same inputs → same outputs.
func TestDecodeCoreDeterminism(t *testing.T) {
	for trial := 0; trial < 3; trial++ {
		s1, c1 := setupCoreState(16, silk.SigTypeUnvoiced)
		s2, c2 := setupCoreState(16, silk.SigTypeUnvoiced)
		q := make([]int32, s1.FrameLength)
		for i := int32(0); i < s1.FrameLength; i += 8 {
			q[i] = int32(i % 7)
		}
		xq1 := make([]int16, s1.FrameLength)
		xq2 := make([]int16, s2.FrameLength)
		DecodeCore(s1, c1, xq1, q)
		DecodeCore(s2, c2, xq2, q)
		for i := range xq1 {
			if xq1[i] != xq2[i] {
				t.Errorf("trial %d idx %d: %d vs %d", trial, i, xq1[i], xq2[i])
				break
			}
		}
	}
}

// TestDecodeShortTermPredictionZeroAR — with all A_Q12 = 0, vec_Q10 = pres_Q10.
func TestDecodeShortTermPredictionZeroAR(t *testing.T) {
	const sub = 16
	pres := []int32{1000, -2000, 3000, -1500, 500, 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100}
	vec := make([]int32, sub)
	sLPC := make([]int32, silk.MaxLPCOrder+sub)
	A := make([]int16, silk.MaxLPCOrder)
	decodeShortTermPrediction(vec, pres, sLPC, A, silk.MaxLPCOrder, sub)
	for i, v := range vec {
		if v != pres[i] {
			t.Errorf("idx %d: vec=%d want %d (zero AR)", i, v, pres[i])
		}
	}
	// State update: sLPC_Q14[MaxLPCOrder + i] = vec << 4.
	for i := int32(0); i < sub; i++ {
		want := vec[i] << 4
		if sLPC[silk.MaxLPCOrder+i] != want {
			t.Errorf("state idx %d: %d want %d", i, sLPC[silk.MaxLPCOrder+i], want)
		}
	}
}
