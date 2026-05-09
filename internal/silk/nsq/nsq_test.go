package nsq

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// freshState returns a fully-initialised NSQ state ready for the first
// frame: PrevInvGainQ16 must be non-zero (the C source asserts this and
// the divide path in nsqScaleStates would explode otherwise).
func freshState() *State {
	s := &State{
		PrevInvGainQ16: 65536, // 1.0 in Q16
		LagPrev:        100,
	}
	return s
}

// freshParams — a minimal FrameParams for a 16 kHz, 320-sample frame
// (default unvoiced; the caller flips Sigtype etc as needed).
func freshParams() *FrameParams {
	p := &FrameParams{
		FrameLength:       320,
		SubfrLength:       80,
		PredictLPCOrder:   silk.MaxLPCOrder,
		ShapingLPCOrder:   silk.MaxLPCOrder,
		Sigtype:           silk.SigTypeUnvoiced,
		QuantOffsetType:   0,
		LSFInterpFactorQ2: 4, // disables interp
		LambdaQ10:         512,
		LTPScaleQ14:       16384,
		Seed:              42,
	}
	for i := range p.GainsQ16 {
		p.GainsQ16[i] = 1 << 16
	}
	return p
}

// TestNSQUnvoicedSilence — silence input → all-zero pulses + zero output.
func TestNSQUnvoicedSilence(t *testing.T) {
	s := freshState()
	p := freshParams()

	x := make([]int16, p.FrameLength)
	q := make([]int8, p.FrameLength)
	var pred [2][silk.MaxLPCOrder]int16
	ltp := make([]int16, silk.LTPOrder*silk.NBSubFr)
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)

	NSQ(s, x, q, p, &pred, ltp, ar2)

	for i, v := range q {
		if v != 0 {
			t.Errorf("silence q[%d] = %d, want 0", i, v)
			break
		}
	}
	// Xq slid forward — second half (the freshly produced samples) should
	// also be zero for silence.
	for i := int32(p.FrameLength); i < 2*p.FrameLength; i++ {
		if s.Xq[i-p.FrameLength] != 0 {
			t.Errorf("Xq[%d] = %d, want 0", i-p.FrameLength, s.Xq[i-p.FrameLength])
			break
		}
	}
}

// TestNSQUnvoicedNoiseProducesPulses — feed unvoiced noise through NSQ
// and verify it produces non-zero pulse output.
func TestNSQUnvoicedNoiseProducesPulses(t *testing.T) {
	s := freshState()
	p := freshParams()

	// White-ish input.
	x := make([]int16, p.FrameLength)
	for i := range x {
		x[i] = int16(2000 + 4000*math.Sin(float64(i)*0.37))
	}

	q := make([]int8, p.FrameLength)
	var pred [2][silk.MaxLPCOrder]int16
	// A trivial AR1 predictor → push the input toward residual.
	for k := int32(0); k < 2; k++ {
		pred[k][0] = 100
	}
	ltp := make([]int16, silk.LTPOrder*silk.NBSubFr)
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)

	NSQ(s, x, q, p, &pred, ltp, ar2)

	pulseCount := 0
	for _, v := range q {
		if v != 0 {
			pulseCount++
		}
	}
	if pulseCount == 0 {
		t.Errorf("no pulses produced for noisy input")
	}
}

// TestNSQVoicedRunsCleanly — voiced sigtype with a non-trivial pitch
// lag exercises the re-whitening branch on subframe 0. Verify the run
// completes, sLagPrev is carried, and some pulses are emitted.
func TestNSQVoicedRunsCleanly(t *testing.T) {
	s := freshState()
	p := freshParams()
	p.Sigtype = silk.SigTypeVoiced
	for i := range p.PitchL {
		p.PitchL[i] = 80
	}
	for i := range p.HarmShapeGainQ14 {
		p.HarmShapeGainQ14[i] = 4096
	}

	x := make([]int16, p.FrameLength)
	for i := range x {
		x[i] = int16(2000 * math.Sin(2*math.Pi*float64(i)/64))
	}
	q := make([]int8, p.FrameLength)
	var pred [2][silk.MaxLPCOrder]int16
	ltp := make([]int16, silk.LTPOrder*silk.NBSubFr)
	for k := int32(0); k < silk.NBSubFr; k++ {
		ltp[k*silk.LTPOrder+silk.LTPOrder/2] = 1 << 12 // 0.25 in Q14
	}
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)

	NSQ(s, x, q, p, &pred, ltp, ar2)

	if s.LagPrev != 80 {
		t.Errorf("LagPrev = %d, want 80", s.LagPrev)
	}
	pulses := 0
	for _, v := range q {
		if v != 0 {
			pulses++
		}
	}
	if pulses == 0 {
		t.Errorf("voiced frame produced no pulses")
	}
}

// TestNSQGainAdjustment — switching subframe gains must rescale state and
// produce sane output (no panics, no NaN-equivalents).
func TestNSQGainAdjustment(t *testing.T) {
	s := freshState()
	p := freshParams()
	for i := range p.GainsQ16 {
		// Increasing gains across subframes.
		p.GainsQ16[i] = (1 << 16) * (int32(i) + 1)
	}

	x := make([]int16, p.FrameLength)
	for i := range x {
		x[i] = int16(1500 * math.Sin(float64(i)*0.21))
	}
	q := make([]int8, p.FrameLength)
	var pred [2][silk.MaxLPCOrder]int16
	ltp := make([]int16, silk.LTPOrder*silk.NBSubFr)
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)

	NSQ(s, x, q, p, &pred, ltp, ar2)
	// Verify state is valid: PrevInvGainQ16 must be > 0.
	if s.PrevInvGainQ16 <= 0 {
		t.Errorf("PrevInvGainQ16 = %d, want > 0", s.PrevInvGainQ16)
	}
}
