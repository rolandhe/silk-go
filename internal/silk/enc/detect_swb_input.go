package enc

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dsp"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// DetectSWBInput — SKP_Silk_detect_SWB_input.
//
// Decides whether the API-side input is super-wideband by HP-filtering at
// ~9 kHz (three biquad SOS sections) and integrating energy above the
// cutoff. After enough consecutive high-band energy the SWB flag latches;
// if the user has been speaking for a while without that latching, the WB
// flag latches instead.
//
// Translated from vendor/silk/src/SKP_Silk_detect_SWB_input.c.
func DetectSWBInput(s *DetectSWBState, samplesIn []int16, nSamplesIn int32) {
	hpLen := fix.MinInt(nSamplesIn, silk.MaxFrameLength)
	hpLen = fix.MaxInt(hpLen, 0)

	var inHP8kHz [silk.MaxFrameLength]int16
	dsp.Biquad(samplesIn, tables.SWB_detect_B_HP_Q13[0][:], tables.SWB_detect_A_HP_Q13[0][:],
		s.SHP8kHz[0][:], inHP8kHz[:], hpLen)
	for i := int32(1); i < silk.NBSOS; i++ {
		dsp.Biquad(inHP8kHz[:], tables.SWB_detect_B_HP_Q13[i][:], tables.SWB_detect_A_HP_Q13[i][:],
			s.SHP8kHz[i][:], inHP8kHz[:], hpLen)
	}

	energy, shift := dsp.SumSqrShift(inHP8kHz[:], hpLen)

	// Threshold is HP_8_KHZ_THRES * len, divided down by the energy's own
	// shift so the comparison is in the original Q domain.
	if energy > fix.RShift32(fix.SmulBB(silk.HP8KHzThres, hpLen), shift) {
		s.ConsecSmplsAboveThr += nSamplesIn
		if s.ConsecSmplsAboveThr > silk.ConsecSWBSamplesThres {
			s.SWBDetected = 1
		}
	} else {
		s.ConsecSmplsAboveThr -= nSamplesIn
		s.ConsecSmplsAboveThr = fix.MaxInt(s.ConsecSmplsAboveThr, 0)
	}

	if s.ActiveSpeechMs > silk.WBDetectActiveSpeechMSThres && s.SWBDetected == 0 {
		s.WBDetected = 1
	}
}
