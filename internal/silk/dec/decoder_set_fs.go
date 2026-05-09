package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/nlsf"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// DecoderSetFs — SKP_Silk_decoder_set_fs.
//
// Configure the decoder for a new internal sample rate (kHz). On change
// it resets the LPC/output buffers and points HP filter + NLSF codebook
// pointers at the rate-specific tables.
//
// Translated from vendor/silk/src/SKP_Silk_decoder_set_fs.c.
func DecoderSetFs(s *State, fsKHz int32) {
	if s.FsKHz == fsKHz {
		return
	}
	s.FsKHz = fsKHz
	s.FrameLength = silk.FrameLengthMs * fsKHz
	s.SubfrLength = (silk.FrameLengthMs / silk.NBSubFr) * fsKHz

	if fsKHz == 8 {
		s.LPCOrder = silk.MinLPCOrder
		s.NLSFCB[0] = &nlsf.CB0_10
		s.NLSFCB[1] = &nlsf.CB1_10
	} else {
		s.LPCOrder = silk.MaxLPCOrder
		s.NLSFCB[0] = &nlsf.CB0_16
		s.NLSFCB[1] = &nlsf.CB1_16
	}

	// Reset transient state.
	for i := range s.SLPCQ14 {
		s.SLPCQ14[i] = 0
	}
	for i := range s.OutBuf {
		s.OutBuf[i] = 0
	}
	for i := range s.PrevNLSFQ15 {
		s.PrevNLSFQ15[i] = 0
	}

	s.LagPrev = 100
	s.LastGainIndex = 1
	s.PrevSigtype = 0
	s.FirstFrameAfterReset = 1

	switch fsKHz {
	case 24:
		s.HPA = tables.Dec_A_HP_24[:]
		s.HPB = tables.Dec_B_HP_24[:]
	case 16:
		s.HPA = tables.Dec_A_HP_16[:]
		s.HPB = tables.Dec_B_HP_16[:]
	case 12:
		s.HPA = tables.Dec_A_HP_12[:]
		s.HPB = tables.Dec_B_HP_12[:]
	case 8:
		s.HPA = tables.Dec_A_HP_8[:]
		s.HPB = tables.Dec_B_HP_8[:]
	}
}
