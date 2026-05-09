package enc

import (
	"math/rand"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
)

// TestFindLPCFIXSmoke — feed a noisy sinusoid through Burg+A2NLSF and
// verify the output NLSFs are strictly increasing in (0, 32768) — the
// usual contract for a stable LPC.
func TestFindLPCFIXSmoke(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const lpcOrder = 16
	const subfrLen = 80 + lpcOrder // 16 kHz, 5ms subframe + history
	totalLen := silk.NBSubFr * subfrLen
	x := make([]int16, totalLen)
	for i := 0; i < totalLen; i++ {
		// Slow-varying signal + noise so Burg has something to fit.
		x[i] = int16(2000*rng.NormFloat64()) + int16(8000*float64(i%17)/17.0)
	}

	var prevNLSF [silk.MaxLPCOrder]int32
	var NLSF [silk.MaxLPCOrder]int32
	var interpIdx int32

	FindLPCFIX(NLSF[:], &interpIdx, prevNLSF[:], 0, lpcOrder, x, subfrLen)
	if interpIdx != 4 {
		t.Errorf("interpIdx = %d, want 4 (no interp)", interpIdx)
	}
	for i := int32(0); i < lpcOrder; i++ {
		if NLSF[i] <= 0 || NLSF[i] >= 32768 {
			t.Errorf("NLSF[%d] = %d out of (0, 32768)", i, NLSF[i])
		}
		if i > 0 && NLSF[i] <= NLSF[i-1] {
			t.Errorf("NLSF not strictly increasing at %d: %d <= %d", i, NLSF[i], NLSF[i-1])
		}
	}
}

// TestResidualEnergyFIXNonNegative — energies must be ≥ 0 and a louder
// signal must produce strictly more residual energy than a quieter one.
func TestResidualEnergyFIXNonNegative(t *testing.T) {
	const lpcOrder = 16
	const subfrLen = 80
	const offset = lpcOrder + subfrLen
	x := make([]int16, silk.NBSubFr*offset)
	for i := range x {
		x[i] = int16((i * 31) & 0x3FF) // small white-ish
	}
	// Trivial all-zero AR coefficients → residual = input.
	var aQ12 [2][silk.MaxLPCOrder]int16

	gains := []int32{65536, 65536, 65536, 65536}
	var nrgs [silk.NBSubFr]int32
	var nrgsQ [silk.NBSubFr]int32

	ResidualEnergyFIX(nrgs[:], nrgsQ[:], x, &aQ12, gains, subfrLen, lpcOrder)
	for i, n := range nrgs {
		if n < 0 {
			t.Errorf("subfr %d: nrg = %d, want ≥ 0", i, n)
		}
	}

	// Scale up the input by 4x — residual energies should also scale up.
	for i := range x {
		x[i] *= 4
	}
	var nrgs2 [silk.NBSubFr]int32
	var nrgs2Q [silk.NBSubFr]int32
	ResidualEnergyFIX(nrgs2[:], nrgs2Q[:], x, &aQ12, gains, subfrLen, lpcOrder)

	// Compare in matching Q. nrgs2 is louder so when normalized to the
	// same Q, it should be ≥ nrgs.
	for i := 0; i < silk.NBSubFr; i++ {
		dq := nrgs2Q[i] - nrgsQ[i]
		var a, b int32
		if dq >= 0 {
			a = fix.RShift32(nrgs2[i], dq)
			b = nrgs[i]
		} else {
			a = nrgs2[i]
			b = fix.RShift32(nrgs[i], -dq)
		}
		if a < b {
			t.Errorf("subfr %d: louder input not >= quieter (a=%d b=%d, dq=%d)", i, a, b, dq)
		}
	}
}

// TestProcessGainsFIXBoundsLambda — Lambda_Q10 should land in (0, 2<<10)
// regardless of the noisy inputs.
func TestProcessGainsFIXBoundsLambda(t *testing.T) {
	var s StateFIX
	InitEncoderFIX(&s)
	s.Cmn.APIFsHz = 16000
	s.Cmn.MaxInternalFsKHz = 16
	if rc := ControlEncoderFIX(&s, 20, 0, 0, 1, 22000); rc != 0 {
		t.Fatalf("ControlEncoderFIX returned %d", rc)
	}
	s.SpeechActivityQ8 = 200
	s.Cmn.NStatesDelayedDecision = 2

	var ctrl ControlFIX
	ctrl.Cmn.Sigtype = silk.SigTypeVoiced
	ctrl.LTPredCodGainQ7 = 6 << 7
	ctrl.InputTiltQ15 = 1024
	ctrl.InputQualityQ14 = 12000
	ctrl.CodingQualityQ14 = 8000
	ctrl.CurrentSNRdBQ7 = 40 << 7
	for i := range ctrl.GainsQ16 {
		ctrl.GainsQ16[i] = 1 << 18
	}
	for i := range ctrl.ResNrg {
		ctrl.ResNrg[i] = 1 << 20
		ctrl.ResNrgQ[i] = 0
	}

	ProcessGainsFIX(&s, &ctrl)
	if ctrl.LambdaQ10 <= 0 || ctrl.LambdaQ10 >= 2<<10 {
		t.Errorf("Lambda_Q10 = %d out of (0, 2048)", ctrl.LambdaQ10)
	}
	for i, g := range ctrl.GainsQ16 {
		if g <= 0 {
			t.Errorf("subfr %d gain ≤ 0: %d", i, g)
		}
	}
}

// TestProcessNLSFsFIXSmoke — quantize a strictly-monotone NLSF and verify
// the output PredCoef tables are populated and that the second half is
// non-zero (the codebook always pulls something close).
func TestProcessNLSFsFIXSmoke(t *testing.T) {
	var s StateFIX
	InitEncoderFIX(&s)
	s.Cmn.APIFsHz = 16000
	s.Cmn.MaxInternalFsKHz = 16
	ControlEncoderFIX(&s, 20, 0, 0, 1, 22000)
	s.SpeechActivityQ8 = 100

	var ctrl ControlFIX
	ctrl.Cmn.Sigtype = silk.SigTypeVoiced
	ctrl.Cmn.NLSFInterpCoefQ2 = 4 // disables interp on this call
	ctrl.SparsenessQ8 = 50

	// Reasonable NLSF: roughly evenly spaced in Q15.
	var nlsfQ15 [silk.MaxLPCOrder]int32
	for i := int32(0); i < s.Cmn.PredictLPCOrder; i++ {
		nlsfQ15[i] = (i + 1) * 32768 / (s.Cmn.PredictLPCOrder + 1)
	}
	// Make sure stabilize sees a valid input first.
	nlsf.Stabilize(nlsfQ15[:], s.Cmn.NLSFCB[ctrl.Cmn.Sigtype].NDeltaMinQ15, s.Cmn.PredictLPCOrder)

	ProcessNLSFsFIX(&s, &ctrl, nlsfQ15[:])

	// Both halves should have non-zero coefficients.
	hasNonzero := func(row []int16) bool {
		for _, v := range row {
			if v != 0 {
				return true
			}
		}
		return false
	}
	if !hasNonzero(ctrl.PredCoefQ12[1][:s.Cmn.PredictLPCOrder]) {
		t.Errorf("second-half PredCoefQ12 all zero")
	}
	if !hasNonzero(ctrl.PredCoefQ12[0][:s.Cmn.PredictLPCOrder]) {
		t.Errorf("first-half PredCoefQ12 all zero")
	}
}
