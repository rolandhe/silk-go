package dec

import (
	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// signCDF builds the {0, sign_CDF[i], 65535} 3-entry CDF used by both
// encoder and decoder.
func signCDF(sigtype, quantOffsetType, rateLevelIndex int32) [3]uint16 {
	i := fix.SmulBB(silk.NRateLevels-1, fix.LShift32(sigtype, 1)+quantOffsetType) + rateLevelIndex
	return [3]uint16{0, tables.Sign_CDF[i], 65535}
}

// EncodeSigns — SKP_Silk_encode_signs.
//
// For every non-zero pulse in q[], emit its sign (0 = negative, 1 = positive)
// using the rate-level-specific 2-symbol CDF.
//
// Translated from vendor/silk/src/SKP_Silk_code_signs.c.
func EncodeSigns(rc *rangecoder.State, q []int8, length int32, sigtype, quantOffsetType, rateLevelIndex int32) {
	cdf := signCDF(sigtype, quantOffsetType, rateLevelIndex)
	for i := int32(0); i < length; i++ {
		if q[i] != 0 {
			// SKP_enc_map(a) = (a >> 15) + 1 → -1 / +1 → 0 / 1.
			data := int32(q[i]>>15) + 1
			rc.Encode(data, cdf[:])
		}
	}
}

// DecodeSigns — SKP_Silk_decode_signs.
//
// q[i] enters as a non-negative pulse magnitude (0 means no pulse). For
// each strictly positive entry, decode a sign bit and multiply through.
//
// Translated from vendor/silk/src/SKP_Silk_code_signs.c.
func DecodeSigns(rc *rangecoder.State, q []int32, length int32, sigtype, quantOffsetType, rateLevelIndex int32) {
	cdf := signCDF(sigtype, quantOffsetType, rateLevelIndex)
	for i := int32(0); i < length; i++ {
		if q[i] > 0 {
			data := rc.Decode(cdf[:], 1)
			// SKP_dec_map(a) = (a << 1) - 1 → 0 / 1 → -1 / +1.
			q[i] *= (data << 1) - 1
		}
	}
}
