package dsp

import "github.com/rolandhe/silk-go/internal/silk/fix"

// freqTableQ16 — sine-window step frequencies, indexed by (length/4 - 4).
// Mirrors the static table from SKP_Silk_apply_sine_window.c. Lengths
// supported: 16, 20, 24, … 120 (multiples of 4).
var freqTableQ16 = [27]int16{
	12111, 9804, 8235, 7100, 6239, 5565, 5022, 4575, 4202,
	3885, 3612, 3375, 3167, 2984, 2820, 2674, 2542, 2422,
	2313, 2214, 2123, 2038, 1961, 1889, 1822, 1760, 1702,
}

// ApplySineWindow — SKP_Silk_apply_sine_window.
//
// Applies a half-period sine window (winType=1: 0→π/2 ramp; winType=2:
// π/2→π ramp) to the input signal. Uses the recurrence
// sin(nf) = 2cos(f)·sin((n-1)f) − sin((n-2)f) every two samples. Window
// length must be 16..120 inclusive and a multiple of 4 (the SILK callers
// guarantee this).
//
// Translated from vendor/silk/src/SKP_Silk_apply_sine_window.c. We follow
// the big-endian/portable path of the C source (two SmulWB per pair) so
// we don't need int32 punning on the little-endian fast path.
func ApplySineWindow(pxWin []int16, px []int16, winType, length int32) {
	k := (length >> 2) - 4
	fQ16 := int32(freqTableQ16[k])
	cQ16 := fix.SmulWB(fQ16, -fQ16)

	var S0Q16, S1Q16 int32
	if winType == 1 {
		S0Q16 = 0
		S1Q16 = fQ16 + fix.RShift32(length, 3)
	} else {
		S0Q16 = 1 << 16
		S1Q16 = (1 << 16) + fix.RShift32(cQ16, 1) + fix.RShift32(length, 4)
	}

	for n := int32(0); n < length; n += 4 {
		pxWin[n] = int16(fix.SmulWB(fix.RShift32(S0Q16+S1Q16, 1), int32(px[n])))
		pxWin[n+1] = int16(fix.SmulWB(S1Q16, int32(px[n+1])))
		S0Q16 = fix.SmulWB(S1Q16, cQ16) + fix.LShift32(S1Q16, 1) - S0Q16 + 1
		if S0Q16 > 1<<16 {
			S0Q16 = 1 << 16
		}

		pxWin[n+2] = int16(fix.SmulWB(fix.RShift32(S0Q16+S1Q16, 1), int32(px[n+2])))
		pxWin[n+3] = int16(fix.SmulWB(S0Q16, int32(px[n+3])))
		S1Q16 = fix.SmulWB(S0Q16, cQ16) + fix.LShift32(S0Q16, 1) - S1Q16
		if S1Q16 > 1<<16 {
			S1Q16 = 1 << 16
		}
	}
}
