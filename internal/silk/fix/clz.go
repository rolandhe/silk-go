package fix

import "math/bits"

// Clz16 — count leading zeros of an int16 treated as a 16-bit value.
// Mirrors SKP_Silk_CLZ16: returns 16 when the input is zero.
func Clz16(in int16) int32 {
	if in == 0 {
		return 16
	}
	return int32(bits.LeadingZeros16(uint16(in)))
}

// Clz32 — count leading zeros of an int32 (SKP_Silk_CLZ32).
func Clz32(in int32) int32 {
	if in == 0 {
		return 32
	}
	return int32(bits.LeadingZeros32(uint32(in)))
}

// Clz64 — count leading zeros of an int64 (SKP_Silk_CLZ64).
func Clz64(in int64) int32 {
	if in == 0 {
		return 64
	}
	return int32(bits.LeadingZeros64(uint64(in)))
}

// ClzFrac — SKP_Silk_CLZ_FRAC: returns the number of leading zeros of `in`
// and the 7 bits immediately after the leading 1, treated as a Q7 fraction.
func ClzFrac(in int32) (lz int32, fracQ7 int32) {
	lz = Clz32(in)
	fracQ7 = Ror32(in, 24-lz) & 0x7F
	return
}

// SqrtApprox — fixed-point sqrt approximation (SKP_Silk_SQRT_APPROX).
// Accuracy: < +/-10% above 15, < +/-2.5% above 120.
func SqrtApprox(x int32) int32 {
	if x <= 0 {
		return 0
	}
	lz, frac := ClzFrac(x)
	var y int32
	if lz&1 != 0 {
		y = 32768
	} else {
		y = 46214 // sqrt(2) * 32768
	}
	y >>= uint(lz >> 1)
	y = SmlaWB(y, y, SmulBB(213, frac))
	return y
}

// Norm16 — number of left shifts before overflow for an int16
// (ITU definition with norm(0) == 0). Mirrors SKP_Silk_norm16.
func Norm16(a int16) int32 {
	if int16(a<<1) == 0 {
		return 0
	}
	a32 := int32(a)
	a32 ^= a32 >> 31
	return Clz32(a32) - 17
}

// Norm32 — number of left shifts before overflow for an int32
// (ITU definition with norm(0) == 0). Mirrors SKP_Silk_norm32.
func Norm32(a int32) int32 {
	if int32(uint32(a)<<1) == 0 {
		return 0
	}
	a ^= a >> 31
	return Clz32(a) - 1
}

// Div32VarQ — SKP_DIV32_varQ. Returns a good approximation of (a32 << Qres) / b32.
// Requires b32 != 0 and Qres >= 0.
func Div32VarQ(a32, b32 int32, qres int32) int32 {
	aHeadrm := Clz32(Abs32(a32)) - 1
	a32Nrm := LShift32(a32, aHeadrm)
	bHeadrm := Clz32(Abs32(b32)) - 1
	b32Nrm := LShift32(b32, bHeadrm)
	b32Inv := Div32By16(int32Max>>2, RShift32(b32Nrm, 16))
	result := SmulWB(a32Nrm, b32Inv)
	a32Nrm -= LShiftOvflw32(Smmul(b32Nrm, result), 3)
	result = SmlaWB(result, a32Nrm, b32Inv)
	lshift := 29 + aHeadrm - bHeadrm - qres
	if lshift <= 0 {
		return LShiftSat32(result, -lshift)
	}
	if lshift < 32 {
		return RShift32(result, lshift)
	}
	return 0
}

// Inverse32VarQ — SKP_INVERSE32_varQ. Returns (1 << Qres) / b32.
// Requires b32 != 0, b32 != int32_min, Qres > 0.
func Inverse32VarQ(b32, qres int32) int32 {
	bHeadrm := Clz32(Abs32(b32)) - 1
	b32Nrm := LShift32(b32, bHeadrm)
	b32Inv := Div32By16(int32Max>>2, RShift32(b32Nrm, 16))
	result := LShift32(b32Inv, 16)
	errQ32 := LShiftOvflw32(-SmulWB(b32Nrm, b32Inv), 3)
	result = SmlaWW(result, errQ32, b32Inv)
	lshift := 61 - bHeadrm - qres
	if lshift <= 0 {
		return LShiftSat32(result, -lshift)
	}
	if lshift < 32 {
		return RShift32(result, lshift)
	}
	return 0
}
