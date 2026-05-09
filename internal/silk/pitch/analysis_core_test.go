package pitch

import (
	"math"
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// TestAnalysisCoreSilenceUnvoiced — pure silence is unvoiced.
func TestAnalysisCoreSilenceUnvoiced(t *testing.T) {
	const fsKHz = 16
	const frameLen = silk.PitchEstFrameLengthMs * fsKHz
	signal := make([]int16, frameLen)
	pitchOut := make([]int32, silk.PitchEstNBSubFr)
	var lagIdx, contourIdx, ltpCorr int32

	rc := AnalysisCore(signal, pitchOut, &lagIdx, &contourIdx, &ltpCorr,
		0, 0xFFFF, 1<<14, fsKHz, 1, 0)
	if rc != 1 {
		t.Errorf("silence should be unvoiced, got %d", rc)
	}
	for i, p := range pitchOut {
		if p != 0 {
			t.Errorf("silence pitchOut[%d] = %d, want 0", i, p)
		}
	}
}

// TestAnalysisCorePeriodicVoiced — a strong periodic input should be
// detected as voiced and yield a pitch lag near the period.
func TestAnalysisCorePeriodicVoiced(t *testing.T) {
	const fsKHz = 16
	const frameLen = silk.PitchEstFrameLengthMs * fsKHz
	const periodSamples = 80 // 5ms = 200 Hz at 16 kHz
	signal := make([]int16, frameLen)
	for i := 0; i < frameLen; i++ {
		signal[i] = int16(8000 * math.Sin(2*math.Pi*float64(i)/periodSamples))
	}
	pitchOut := make([]int32, silk.PitchEstNBSubFr)
	var lagIdx, contourIdx, ltpCorr int32

	rc := AnalysisCore(signal, pitchOut, &lagIdx, &contourIdx, &ltpCorr,
		0, 0xFFFF, 1<<14, fsKHz, 1, 0)
	if rc != 0 {
		t.Errorf("strong periodic should be voiced, got %d", rc)
	}
	// Expect pitchOut roughly near periodSamples (within a 4× tolerance —
	// the codebook can land on a multiple of the fundamental).
	for i, p := range pitchOut {
		if p < periodSamples/4 || p > periodSamples*4 {
			t.Errorf("subfr %d: pitch %d not near %d", i, p, periodSamples)
		}
	}
	if ltpCorr <= 0 {
		t.Errorf("LTPCorr_Q15 = %d, want > 0 for voiced", ltpCorr)
	}
}

// TestFindScalingMonotone — louder signals need more right-shift to keep
// inner products from overflowing.
func TestFindScalingMonotone(t *testing.T) {
	signal := make([]int16, 320)
	for i := range signal {
		signal[i] = 1000
	}
	low := findScaling(signal, int32(len(signal)), int32(len(signal)))

	for i := range signal {
		signal[i] = 30000
	}
	high := findScaling(signal, int32(len(signal)), int32(len(signal)))

	if !(high >= low) {
		t.Errorf("findScaling not monotone: low=%d high=%d", low, high)
	}
}
