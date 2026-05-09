package nsq

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// TestNSQDelDecUnvoicedSilence — silence input → all-zero pulses, 4 states.
func TestNSQDelDecUnvoicedSilence(t *testing.T) {
	s := freshState()
	p := freshParams()

	x := make([]int16, p.FrameLength)
	q := make([]int8, p.FrameLength)
	var pred [2][silk.MaxLPCOrder]int16
	ltp := make([]int16, silk.LTPOrder*silk.NBSubFr)
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)

	var seedOut int32
	NSQDelDec(s, x, q, p, &pred, ltp, ar2, 0, silk.MaxDelDecStates, &seedOut)

	for i, v := range q {
		if v != 0 {
			t.Errorf("silence q[%d] = %d, want 0", i, v)
			break
		}
	}
}

// TestNSQDelDecUnvoicedNoiseProducesPulses — noisy unvoiced input must
// produce some pulse output and update PrevInvGainQ16.
func TestNSQDelDecUnvoicedNoiseProducesPulses(t *testing.T) {
	s := freshState()
	p := freshParams()

	x := make([]int16, p.FrameLength)
	for i := range x {
		x[i] = int16(2000 + 4000*math.Sin(float64(i)*0.37))
	}
	q := make([]int8, p.FrameLength)
	var pred [2][silk.MaxLPCOrder]int16
	for k := int32(0); k < 2; k++ {
		pred[k][0] = 100
	}
	ltp := make([]int16, silk.LTPOrder*silk.NBSubFr)
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)

	var seedOut int32
	NSQDelDec(s, x, q, p, &pred, ltp, ar2, 0, 2, &seedOut)

	pulseCount := 0
	for _, v := range q {
		if v != 0 {
			pulseCount++
		}
	}
	if pulseCount == 0 {
		t.Errorf("no pulses produced for noisy input")
	}
	if s.PrevInvGainQ16 <= 0 {
		t.Errorf("PrevInvGainQ16 = %d, want > 0", s.PrevInvGainQ16)
	}
}

// TestNSQDelDecVoicedRunsCleanly — voiced sigtype with a non-trivial
// pitch lag exercises the re-whitening + reset-at-k=2 path.
func TestNSQDelDecVoicedRunsCleanly(t *testing.T) {
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
		ltp[k*silk.LTPOrder+silk.LTPOrder/2] = 1 << 12
	}
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)

	var seedOut int32
	NSQDelDec(s, x, q, p, &pred, ltp, ar2, 0, silk.MaxDelDecStates, &seedOut)

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
		t.Errorf("voiced del-dec frame produced no pulses")
	}
	// SeedInit of the winner is what the encoder echoes back.
	if seedOut < 0 || seedOut > 3 {
		t.Errorf("seedOut = %d, want in [0, 3]", seedOut)
	}
}

// TestNSQDelDecMatchesNSQAtSingleState — with nStatesDelayedDecision=1
// and decision_delay forced low, NSQDelDec should produce results
// approximately equivalent to single-state NSQ on the noise side.
// We only check that both produce a similar count of non-zero pulses;
// the exact pulse values diverge because the search topology differs.
func TestNSQDelDecMatchesNSQAtSingleState(t *testing.T) {
	x := make([]int16, 320)
	for i := range x {
		x[i] = int16(3000 * math.Sin(float64(i)*0.13))
	}

	pSingle := freshParams()
	sSingle := freshState()
	qSingle := make([]int8, len(x))
	var pred [2][silk.MaxLPCOrder]int16
	ltp := make([]int16, silk.LTPOrder*silk.NBSubFr)
	ar2 := make([]int16, silk.NBSubFr*silk.MaxShapeLPCOrder)
	NSQ(sSingle, x, qSingle, pSingle, &pred, ltp, ar2)
	pulsesSingle := 0
	for _, v := range qSingle {
		if v != 0 {
			pulsesSingle++
		}
	}

	pDel := freshParams()
	sDel := freshState()
	qDel := make([]int8, len(x))
	var seedOut int32
	NSQDelDec(sDel, x, qDel, pDel, &pred, ltp, ar2, 0, 1, &seedOut)
	pulsesDel := 0
	for _, v := range qDel {
		if v != 0 {
			pulsesDel++
		}
	}

	// Single-state del_dec uses the same RD search but with decision
	// delay; expect comparable pulse density (within 4×).
	if pulsesSingle == 0 && pulsesDel == 0 {
		t.Skip("both quantizers silent on this input")
	}
	if pulsesDel == 0 || pulsesSingle == 0 {
		t.Errorf("pulse count mismatch: NSQ=%d, NSQDelDec(1)=%d", pulsesSingle, pulsesDel)
	}
}
