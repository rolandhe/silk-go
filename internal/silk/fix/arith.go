// Package fix implements the SILK fixed-point arithmetic primitives ported
// from vendor/silk/src/SKP_Silk_macros.h, SKP_Silk_Inlines.h and
// SKP_Silk_SigProc_FIX.h.
//
// The functions are deliberately small and pure so the Go compiler inlines
// them on the hot path. Names follow the C originals (SKP_/SKP_Silk_ stripped,
// camelCase to Go style) and each one is annotated with the C source line it
// was translated from.
//
// All shift macros assume the C convention: shift counts are non-negative
// and strictly less than the operand width. The caller is responsible for
// guaranteeing that. Overflow-allowed variants (LShiftOvflw, AddOvflw, etc.)
// model two's-complement wrap by routing through uint32/uint64; signed-shift
// of a negative value is performed via uint conversion to avoid Go panics on
// shift-out-of-range and to match the wrap behavior the C codebase relies on.
package fix

// ---------------------------------------------------------------------------
// Min/max/limit/abs (SKP_min*, SKP_max*, SKP_LIMIT*, SKP_abs).
// ---------------------------------------------------------------------------

func MinInt(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func MaxInt(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func Min32(a, b int32) int32 { return MinInt(a, b) }
func Max32(a, b int32) int32 { return MaxInt(a, b) }

func Min16(a, b int16) int16 {
	if a < b {
		return a
	}
	return b
}

func Max16(a, b int16) int16 {
	if a > b {
		return a
	}
	return b
}

func Min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func Max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// Limit clamps a into [min(l1,l2), max(l1,l2)] (mirrors SKP_LIMIT).
func Limit(a, l1, l2 int32) int32 {
	if l1 > l2 {
		if a > l1 {
			return l1
		}
		if a < l2 {
			return l2
		}
		return a
	}
	if a > l2 {
		return l2
	}
	if a < l1 {
		return l1
	}
	return a
}

// Abs32 mirrors SKP_abs; behaves as |a| for int32 except a == math.MinInt32
// stays MinInt32 (same as the C original which also breaks on int32_MIN).
func Abs32(a int32) int32 {
	if a < 0 {
		return -a
	}
	return a
}

// AbsInt32 mirrors SKP_abs_int32 — branch-free; same MinInt32 caveat.
func AbsInt32(a int32) int32 {
	mask := a >> 31
	return (a ^ mask) - mask
}

func Abs64(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
}

// ---------------------------------------------------------------------------
// Saturation (SKP_SAT16/32, SKP_ADD_SAT16, SKP_ADD_POS_SAT32, SKP_ADD_SAT32, SKP_SUB_SAT32).
// ---------------------------------------------------------------------------

const (
	int16Max = int32(1<<15 - 1)
	int16Min = int32(-1 << 15)
	int32Max = int32(1<<31 - 1)
	int32Min = int32(-1 << 31)
)

func Sat16(a int32) int32 {
	if a > int16Max {
		return int16Max
	}
	if a < int16Min {
		return int16Min
	}
	return a
}

func Sat32From64(a int64) int32 {
	if a > int64(int32Max) {
		return int32Max
	}
	if a < int64(int32Min) {
		return int32Min
	}
	return int32(a)
}

func AddSat16(a, b int32) int16 { return int16(Sat16(a + b)) }

// AddPosSat32: SKP_ADD_POS_SAT32 — used when a, b are known non-negative;
// if their sum overflows into the sign bit, return INT32_MAX.
func AddPosSat32(a, b int32) int32 {
	s := a + b
	if uint32(s)&0x80000000 != 0 {
		return int32Max
	}
	return s
}

// AddSat32 saturates an int32+int32 sum (SKP_ADD_SAT32 in macros.h).
func AddSat32(a, b int32) int32 {
	s := int64(a) + int64(b)
	return Sat32From64(s)
}

// SubSat32 saturates an int32-int32 difference (SKP_SUB_SAT32).
func SubSat32(a, b int32) int32 {
	s := int64(a) - int64(b)
	return Sat32From64(s)
}

// ---------------------------------------------------------------------------
// Shifts (SKP_LSHIFT/RSHIFT(32|64), SKP_LSHIFT_ovflw, SKP_LSHIFT_SAT32, SKP_RSHIFT_ROUND).
// ---------------------------------------------------------------------------

// LShift32 is the safe variant: callers must guarantee 0 <= shift < 32 and
// the result fits in int32. For overflow-allowed shifts use LShiftOvflw32.
func LShift32(a, shift int32) int32 { return a << uint(shift) }

func LShift64(a int64, shift int32) int64 { return a << uint(shift) }

func RShift32(a, shift int32) int32 { return a >> uint(shift) }

func RShift64(a int64, shift int32) int64 { return a >> uint(shift) }

// LShiftOvflw32 mirrors SKP_LSHIFT_ovflw / SKP_LSHIFT_uint: bits shifted out
// are silently dropped, the result is reinterpreted in two's complement.
func LShiftOvflw32(a, shift int32) int32 {
	return int32(uint32(a) << uint(shift))
}

func LShiftOvflw64(a int64, shift int32) int64 {
	return int64(uint64(a) << uint(shift))
}

// LShiftSat32 — SKP_LSHIFT_SAT32. Matches the C macro semantics exactly:
//
//	LSHIFT_SAT32(a, sh) = LIMIT_32(a, INT32_MIN >> sh, INT32_MAX >> sh) << sh
//
// The result of clamping then shifting is NOT INT32_MAX/MIN at the
// boundary — it's `(INT32_MAX >> sh) << sh`, which has the low `sh`
// bits zeroed. Earlier shortcuts that returned INT32_MAX/MIN diverged
// from C for the typical "result almost overflows" case.
//
// Precondition (C): 0 <= shift < 32. We tolerate shift <= 0 as a no-op
// to keep callers benign; shift >= 32 is undefined in both.
func LShiftSat32(a, shift int32) int32 {
	if shift <= 0 {
		return a
	}
	hi := int32Max >> uint(shift)
	lo := int32Min >> uint(shift)
	if a > hi {
		a = hi
	} else if a < lo {
		a = lo
	}
	return a << uint(shift)
}

// RShiftRound — SKP_RSHIFT_ROUND. Requires shift > 0.
func RShiftRound(a, shift int32) int32 {
	if shift == 1 {
		return (a >> 1) + (a & 1)
	}
	return ((a >> uint(shift-1)) + 1) >> 1
}

func RShiftRound64(a int64, shift int32) int64 {
	if shift == 1 {
		return (a >> 1) + (a & 1)
	}
	return ((a >> uint(shift-1)) + 1) >> 1
}

// AddLShift32 / AddRShift32 / SubLShift32 / SubRShift32 mirror the SKP_*_LSHIFT/RSHIFT
// helper macros — non-overflowing add/sub combined with a shift.
func AddLShift32(a, b, shift int32) int32 { return a + (b << uint(shift)) }
func AddRShift32(a, b, shift int32) int32 { return a + (b >> uint(shift)) }
func SubLShift32(a, b, shift int32) int32 { return a - (b << uint(shift)) }
func SubRShift32(a, b, shift int32) int32 { return a - (b >> uint(shift)) }

// ---------------------------------------------------------------------------
// Add/sub with wrap (SKP_ADD32_ovflw, SKP_SUB32_ovflw, SKP_MLA_ovflw and friends).
// ---------------------------------------------------------------------------

func AddOvflw32(a, b int32) int32 { return int32(uint32(a) + uint32(b)) }
func SubOvflw32(a, b int32) int32 { return int32(uint32(a) - uint32(b)) }

// Add32/Sub32/Add64 are the plain (non-saturating) helpers; here for parity
// with the macro names.
func Add32(a, b int32) int32 { return a + b }
func Sub32(a, b int32) int32 { return a - b }
func Add64(a, b int64) int64 { return a + b }

// ---------------------------------------------------------------------------
// Multiply / multiply-accumulate (SKP_MUL, SKP_MLA, SKP_SMUL**, SKP_SMLA**).
// ---------------------------------------------------------------------------

func Mul(a, b int32) int32    { return a * b }
func Mla(a, b, c int32) int32 { return a + b*c }

// MlaOvflw — SKP_MLA_ovflw.
func MlaOvflw(a, b, c int32) int32 {
	return int32(uint32(a) + uint32(b)*uint32(c))
}

// SmulWB — (a32 * (int16)b32) with the upper-half/lower-half decomposition
// from SKP_SMULWB. The result is int32 but uses int64 internally to avoid
// signed overflow during the intermediate multiply.
func SmulWB(a, b int32) int32 {
	bs := int32(int16(b))
	hi := (a >> 16) * bs
	lo := ((a & 0xFFFF) * bs) >> 16
	return hi + lo
}

// SmlaWB — a + SmulWB(b, c).
func SmlaWB(a, b, c int32) int32 { return a + SmulWB(b, c) }

// SmulWT — (a32 * (b32 >> 16)) >> 16 form.
func SmulWT(a, b int32) int32 {
	hi := (a >> 16) * (b >> 16)
	lo := ((a & 0xFFFF) * (b >> 16)) >> 16
	return hi + lo
}

// SmlaWT — a + SmulWT(b, c).
func SmlaWT(a, b, c int32) int32 { return a + SmulWT(b, c) }

// SmulBB — (int16)a * (int16)b, result int32.
func SmulBB(a, b int32) int32 { return int32(int16(a)) * int32(int16(b)) }

// SmlaBB — a + SmulBB(b, c).
func SmlaBB(a, b, c int32) int32 { return a + SmulBB(b, c) }

// SmulBT — (int16)a * (b >> 16).
func SmulBT(a, b int32) int32 { return int32(int16(a)) * (b >> 16) }

// SmlaBT — a + SmulBT(b, c).
func SmlaBT(a, b, c int32) int32 { return a + SmulBT(b, c) }

// SmulTT — (a >> 16) * (b >> 16).
func SmulTT(a, b int32) int32 { return (a >> 16) * (b >> 16) }

// SmlaTT — a + SmulTT(b, c).
func SmlaTT(a, b, c int32) int32 { return a + SmulTT(b, c) }

// SmlaTTOvflw — a + SmulTT(b, c) with two's-complement wrap.
func SmlaTTOvflw(a, b, c int32) int32 {
	return int32(uint32(a) + uint32(SmulTT(b, c)))
}

// SmlaWBOvflw / SmlaWTOvflw — overflow-allowed variants.
func SmlaWBOvflw(a, b, c int32) int32 {
	return int32(uint32(a) + uint32(SmulWB(b, c)))
}

func SmlaWTOvflw(a, b, c int32) int32 {
	return int32(uint32(a) + uint32(SmulWT(b, c)))
}

// SmlaBBOvflw — a + SmulBB(b, c) with two's-complement wrap.
func SmlaBBOvflw(a, b, c int32) int32 {
	return int32(uint32(a) + uint32(SmulBB(b, c)))
}

// Smull — (int64)a * (int64)b (SKP_SMULL).
func Smull(a, b int32) int64 { return int64(a) * int64(b) }

// Smlal — a64 + (int64)b * (int64)c.
func Smlal(a int64, b, c int32) int64 { return a + int64(b)*int64(c) }

// SmlalBB — a64 + (int32)((int16)b * (int16)c).
func SmlalBB(a int64, b, c int32) int64 {
	return a + int64(int32(int16(b))*int32(int16(c)))
}

// SmulWW — (a32 * b32) >> 16 (SKP_SMULWW = SmulWB(a,b) + a*RShiftRound(b,16)).
func SmulWW(a, b int32) int32 {
	return SmlaWB(0, a, b) + a*RShiftRound(b, 16)
}

// SmlaWW — a + SmulWW(b, c) (SKP_SMLAWW = SmlaWB(a,b,c) + b*RShiftRound(c,16)).
func SmlaWW(a, b, c int32) int32 {
	return SmlaWB(a, b, c) + b*RShiftRound(c, 16)
}

// Smmul — top 32 bits of (int64)a*b (SKP_SMMUL).
func Smmul(a, b int32) int32 { return int32(Smull(a, b) >> 32) }

// ---------------------------------------------------------------------------
// Division (SKP_DIV32, SKP_DIV32_16). The macros are plain integer division;
// callers are responsible for non-zero divisor.
// ---------------------------------------------------------------------------

func Div32(a, b int32) int32     { return a / b }
func Div32By16(a, b int32) int32 { return a / b }

// ---------------------------------------------------------------------------
// FixConst — SKP_FIX_CONST(C, Q) = (int32)(C * (1 << Q) + 0.5).
// Callers should ensure the resulting constant fits in int32.
// ---------------------------------------------------------------------------

// FixConst32 — SKP_FIX_CONST(C, Q) = (int32)((C) * ((int64)1 << Q) + 0.5).
//
// Note: the +0.5 bias is unconditional, even for negative C. C89/C99
// float→int conversion truncates toward zero, so the rounding direction
// for negative values is "round toward +∞": -2.7+0.5 = -2.2 → -2 (NOT -3).
// Earlier versions of this helper had `if v >= 0 { v+0.5 } else { v-0.5 }`
// which rounded away from zero on negatives — that diverged from C and
// was the dominant cause of encoder byte-drift vs the C reference.
func FixConst32(c float64, q uint) int32 {
	return int32(c*float64(int64(1)<<q) + 0.5)
}

// Ror32 — rotate right by `rot` bits; negative rot rotates left.
func Ror32(a, rot int32) int32 {
	x := uint32(a)
	r := uint32(rot)
	if rot <= 0 {
		m := uint32(-rot)
		return int32((x << m) | (x >> (32 - m)))
	}
	return int32((x << (32 - r)) | (x >> r))
}

// Rand — SKP_RAND. Returns the next pseudo-random seed.
func Rand(seed int32) int32 { return MlaOvflw(907633515, seed, 196314165) }
