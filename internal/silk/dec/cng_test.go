package dec

import (
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// TestCNGResetSpacesNLSF — after CNGReset, the smoothed NLSF vector should
// be evenly spaced from 0 to ~int16_max.
func TestCNGResetSpacesNLSF(t *testing.T) {
	psDec := &State{}
	psDec.LPCOrder = 10
	CNGReset(psDec)

	prev := int32(0)
	for i := int32(0); i < psDec.LPCOrder; i++ {
		v := psDec.SCNG.CNGSmthNLSFQ15[i]
		if v <= prev {
			t.Errorf("NLSF[%d]=%d not strictly above prev %d", i, v, prev)
		}
		prev = v
	}
	if psDec.SCNG.CNGSmthGainQ16 != 0 {
		t.Errorf("CNGSmthGainQ16=%d want 0", psDec.SCNG.CNGSmthGainQ16)
	}
	if psDec.SCNG.RandSeed != 3176576 {
		t.Errorf("RandSeed=%d want 3176576", psDec.SCNG.RandSeed)
	}
}

// TestCNGNoLossNoEffect — when LossCnt is 0, CNG must not modify signal[].
func TestCNGNoLossNoEffect(t *testing.T) {
	psDec := &State{}
	psDec.LPCOrder = 10
	psDec.FsKHz = 8
	psDec.SubfrLength = 40
	psDec.LossCnt = 0
	psDec.VadFlag = silk.VoiceActivity // not low activity → no smoothing either
	ctrl := &Control{}
	for i := range ctrl.GainsQ16 {
		ctrl.GainsQ16[i] = 1 << 16
	}
	signal := []int16{100, 200, -300, 400, -500}
	original := append([]int16(nil), signal...)

	CNG(psDec, ctrl, signal, int32(len(signal)))
	for i, v := range signal {
		if v != original[i] {
			t.Errorf("idx %d: signal modified %d → %d (no-loss path)", i, original[i], v)
		}
	}
}

// TestCNGUpdatesBufferOnLowActivity — vadFlag == 0 and lossCnt == 0 should
// drive smoothed gain non-zero after enough frames.
func TestCNGUpdatesGainOnLowActivity(t *testing.T) {
	psDec := &State{}
	psDec.LPCOrder = 10
	psDec.FsKHz = 8
	psDec.SubfrLength = 40
	psDec.LossCnt = 0
	psDec.VadFlag = silk.NoVoiceActivity
	ctrl := &Control{}
	for i := range ctrl.GainsQ16 {
		ctrl.GainsQ16[i] = 100000 // arbitrary positive value
	}

	signal := make([]int16, 160)
	// Run several frames so the smoothing accumulates.
	for n := 0; n < 50; n++ {
		CNG(psDec, ctrl, signal, 160)
	}
	if psDec.SCNG.CNGSmthGainQ16 == 0 {
		t.Errorf("CNGSmthGainQ16 still 0 after 50 frames of low activity")
	}
}

// TestCNGOnLossMixesNonZero — when LossCnt > 0, CNG should add a non-zero
// contribution to the (zeroed) signal.
func TestCNGOnLossMixesNonZero(t *testing.T) {
	psDec := &State{}
	psDec.LPCOrder = 10
	psDec.FsKHz = 8
	psDec.SubfrLength = 40
	// First populate the CNG buffer through a few low-activity frames.
	psDec.LossCnt = 0
	psDec.VadFlag = silk.NoVoiceActivity
	ctrl := &Control{}
	for i := range ctrl.GainsQ16 {
		ctrl.GainsQ16[i] = 1 << 18
	}
	// Excitation needs to be loud enough that SmulWW(exc, smthGain)>>10
	// doesn't round all-zero in cngExc.
	for i := range psDec.ExcQ10 {
		psDec.ExcQ10[i] = int32((i%200)*100 - 10000)
	}
	for i := range psDec.PrevNLSFQ15 {
		psDec.PrevNLSFQ15[i] = (int32(i) + 1) * 2000
	}
	signal := make([]int16, 160)
	for n := 0; n < 10; n++ {
		CNG(psDec, ctrl, signal, 160)
	}

	// Now flip into loss state and reset the input signal.
	psDec.LossCnt = 1
	for i := range signal {
		signal[i] = 0
	}
	CNG(psDec, ctrl, signal, 160)
	allZero := true
	for _, v := range signal {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Errorf("CNG on loss produced all-zero output")
	}
}
