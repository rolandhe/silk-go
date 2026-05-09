package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// Lin2Log — SKP_Silk_lin2log.
//
// Approximation of 128 * log2(inLin). Returns a Q7 log-scale value with a
// piecewise-parabolic correction over the fractional part of the leading 1.
//
// Translated from vendor/silk/src/SKP_Silk_lin2log.c.
func Lin2Log(inLin int32) int32 {
	lz, fracQ7 := fix.ClzFrac(inLin)
	return fix.LShift32(31-lz, 7) +
		fix.SmlaWB(fracQ7, fix.Mul(fracQ7, 128-fracQ7), 179)
}

// Log2Lin — SKP_Silk_log2lin.
//
// Approximation of 2^(inLog/128) — inverse of Lin2Log. Saturates at 0 for
// negative input and at int32_max for input ≥ 31<<7.
//
// Translated from vendor/silk/src/SKP_Silk_log2lin.c.
func Log2Lin(inLogQ7 int32) int32 {
	if inLogQ7 < 0 {
		return 0
	}
	if inLogQ7 >= 31<<7 {
		return 0x7FFFFFFF
	}
	out := fix.LShift32(1, fix.RShift32(inLogQ7, 7))
	fracQ7 := inLogQ7 & 0x7F
	corr := fix.SmlaWB(fracQ7, fix.Mul(fracQ7, 128-fracQ7), -174)
	if inLogQ7 < 2048 {
		out = fix.AddRShift32(out, fix.Mul(out, corr), 7)
	} else {
		out = fix.Mla(out, fix.RShift32(out, 7), corr)
	}
	return out
}
