package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// BwExpander — SKP_Silk_bwexpander.
// In-place chirp (bandwidth expand) on an int16 LP-AR filter, using
// RShiftRound(MUL(...), 16) instead of SmulWB to avoid the rounding bias.
//
// Translated from vendor/silk/src/SKP_Silk_bwexpander.c.
func BwExpander(ar []int16, d int32, chirpQ16 int32) {
	chirpMinusOneQ16 := chirpQ16 - 65536
	for i := int32(0); i < d-1; i++ {
		ar[i] = int16(fix.RShiftRound(chirpQ16*int32(ar[i]), 16))
		chirpQ16 += fix.RShiftRound(chirpQ16*chirpMinusOneQ16, 16)
	}
	ar[d-1] = int16(fix.RShiftRound(chirpQ16*int32(ar[d-1]), 16))
}

// BwExpander32 — SKP_Silk_bwexpander_32.
// In-place chirp on an int32 LP-AR filter, using SmulWW.
//
// Translated from vendor/silk/src/SKP_Silk_bwexpander_32.c.
func BwExpander32(ar []int32, d int32, chirpQ16 int32) {
	tmpChirpQ16 := chirpQ16
	for i := int32(0); i < d-1; i++ {
		ar[i] = fix.SmulWW(ar[i], tmpChirpQ16)
		tmpChirpQ16 = fix.SmulWW(chirpQ16, tmpChirpQ16)
	}
	ar[d-1] = fix.SmulWW(ar[d-1], tmpChirpQ16)
}
