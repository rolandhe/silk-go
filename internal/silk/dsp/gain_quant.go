package dsp

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
)

// Gain quantization parameters — see SKP_Silk_define.h:
//
//	MIN_QGAIN_DB = 6, MAX_QGAIN_DB = 86, N_LEVELS_QGAIN = 64.
//	OFFSET = (MIN_QGAIN_DB * 128 / 6) + 16 * 128 = 2176
//	SCALE_Q16 = 65536 * 63 / ((86 - 6) * 128 / 6) = 2418
//	INV_SCALE_Q16 = 65536 * ((86-6)*128/6) / 63 = 1775
const (
	gainOffset      = ((silk.MinQGainDB * 128) / 6) + 16*128
	gainScaleQ16    = (65536 * (silk.NLevelsQGain - 1)) / ((silk.MaxQGainDB - silk.MinQGainDB) * 128 / 6)
	gainInvScaleQ16 = 65536 * ((silk.MaxQGainDB - silk.MinQGainDB) * 128 / 6) / (silk.NLevelsQGain - 1)
)

// GainsQuant — SKP_Silk_gains_quant.
//
// Encoder-side gain quantizer with hysteresis. ind[] receives the
// per-subframe indices; gainQ16[] is updated in place to the quantized
// gains. prevInd carries the last index from the previous frame.
//
// Translated from vendor/silk/src/SKP_Silk_gain_quant.c.
func GainsQuant(ind []int32, gainQ16 []int32, prevInd *int32, conditional int32) {
	for k := int32(0); k < silk.NBSubFr; k++ {
		ind[k] = fix.SmulWB(gainScaleQ16, Lin2Log(gainQ16[k])-gainOffset)
		if ind[k] < *prevInd {
			ind[k]++
		}
		if k == 0 && conditional == 0 {
			ind[k] = fix.Limit(ind[k], 0, silk.NLevelsQGain-1)
			if ind[k] < *prevInd+silk.MinDeltaGainQuant {
				ind[k] = *prevInd + silk.MinDeltaGainQuant
			}
			*prevInd = ind[k]
		} else {
			ind[k] = fix.Limit(ind[k]-*prevInd, silk.MinDeltaGainQuant, silk.MaxDeltaGainQuant)
			*prevInd += ind[k]
			ind[k] -= silk.MinDeltaGainQuant
		}
		v := fix.SmulWB(gainInvScaleQ16, *prevInd) + gainOffset
		if v > 3967 {
			v = 3967
		}
		gainQ16[k] = Log2Lin(v)
	}
}

// GainsDequant — SKP_Silk_gains_dequant.
//
// Decoder-side gain dequantizer; reads ind[] and writes gainQ16[].
//
// Translated from vendor/silk/src/SKP_Silk_gain_quant.c.
func GainsDequant(gainQ16 []int32, ind []int32, prevInd *int32, conditional int32) {
	for k := int32(0); k < silk.NBSubFr; k++ {
		if k == 0 && conditional == 0 {
			*prevInd = ind[k]
		} else {
			*prevInd += ind[k] + silk.MinDeltaGainQuant
		}
		v := fix.SmulWB(gainInvScaleQ16, *prevInd) + gainOffset
		if v > 3967 {
			v = 3967
		}
		gainQ16[k] = Log2Lin(v)
	}
}
