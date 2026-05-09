package enc

import (
	"github.com/rolandhe/silk-go/internal/silk/vad"
)

// InitEncoderFIX — SKP_Silk_init_encoder_FIX.
//
// Zeros the encoder state, primes the variable-HP cutoff smoothers to
// log2(70) (Q15), arms the FirstFrameAfterReset flag (so NLSF interpolation
// and fluctuation reduction stay off for one frame), initializes the VAD,
// and sets PrevInvGainQ16 = 1.0 in Q16 for both NSQ instances.
//
// Translated from vendor/silk/src/SKP_Silk_init_encoder_FIX.c.
func InitEncoderFIX(psEnc *StateFIX) int32 {
	*psEnc = StateFIX{}

	psEnc.VariableHPSmth1Q15 = 200844 // log2(70) in Q15
	psEnc.VariableHPSmth2Q15 = 200844

	psEnc.Cmn.FirstFrameAfterReset = 1

	ret := vad.Init(&psEnc.Cmn.SVAD)

	psEnc.Cmn.SNSQ.PrevInvGainQ16 = 65536
	psEnc.Cmn.SNSQLBRR.PrevInvGainQ16 = 65536

	return ret
}
