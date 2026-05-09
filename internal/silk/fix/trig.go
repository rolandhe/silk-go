package fix

const (
	sinApproxConst0 = int32(1073735400)
	sinApproxConst1 = int32(-82778932)
	sinApproxConst2 = int32(1059577)
	sinApproxConst3 = int32(-5013)
)

// SinApproxQ24 — SKP_Silk_SIN_APPROX_Q24.
// Returns approximately 2^24 * sin(x * 2π / 65536). Relative error < 1e-5.
func SinApproxQ24(x int32) int32 {
	x &= 65535
	if x <= 32768 {
		if x < 16384 {
			x = 16384 - x
		} else {
			x -= 16384
		}
		if x < 1100 {
			return SmlaWB(1<<24, Mul(x, x), -5053)
		}
		x = SmulWB(LShift32(x, 8), x)
		yQ30 := SmlaWB(sinApproxConst2, x, sinApproxConst3)
		yQ30 = SmlaWW(sinApproxConst1, x, yQ30)
		yQ30 = SmlaWW(sinApproxConst0+66, x, yQ30)
		return RShiftRound(yQ30, 6)
	}
	if x < 49152 {
		x = 49152 - x
	} else {
		x -= 49152
	}
	if x < 1100 {
		return SmlaWB(-(1 << 24), Mul(x, x), 5053)
	}
	x = SmulWB(LShift32(x, 8), x)
	yQ30 := SmlaWB(-sinApproxConst2, x, -sinApproxConst3)
	yQ30 = SmlaWW(-sinApproxConst1, x, yQ30)
	yQ30 = SmlaWW(-sinApproxConst0, x, yQ30)
	return RShiftRound(yQ30, 6)
}

// CosApproxQ24 — SKP_Silk_COS_APPROX_Q24.
func CosApproxQ24(x int32) int32 { return SinApproxQ24(x + 16384) }
