package ltp

import (
	"math"
	"math/rand"
	"testing"
)

// preLagMargin is the extra precedence the FIR consumes beyond pitchL: the
// 5-tap centred at lag means we read down to xBuf[lagOff + LTPOrder/2 - (LTPOrder-1)]
// = xBuf[lagOff - 2]. Real SILK avoids this by guaranteeing pitchL ≥ ~48.
const preLagMargin = LTPOrder

// TestAnalysisFilterZeroCoef — when LTP coefficients are all zero, the
// "predicted" component vanishes, so residual = invGain * x (scaled).
func TestAnalysisFilterZeroCoef(t *testing.T) {
	subfrLen := int32(40)
	preLen := int32(20)
	pitchL := []int32{30, 30, 30, 30}
	maxLag := int32(0)
	for _, p := range pitchL {
		if p > maxLag {
			maxLag = p
		}
	}
	xStart := maxLag + preLagMargin
	xLen := xStart + NBSubFr*subfrLen + preLen
	x := make([]int16, xLen)
	rng := rand.New(rand.NewSource(1))
	for i := range x {
		x[i] = int16(rng.Intn(20001) - 10000)
	}
	coef := make([]int16, LTPOrder*NBSubFr)
	gains := []int32{1 << 16, 1 << 16, 1 << 16, 1 << 16}

	res := make([]int16, NBSubFr*(subfrLen+preLen))
	AnalysisFilter(res, x, xStart, coef, pitchL, gains, subfrLen, preLen)

	// With zero coefficients, ltpEst = 0, residual = SmulWB(1<<16, x[i]) ≈ x[i].
	for k := int32(0); k < NBSubFr; k++ {
		for i := int32(0); i < subfrLen+preLen; i++ {
			want := x[xStart+k*subfrLen+i]
			got := res[k*(subfrLen+preLen)+i]
			if got != want && got != want-1 && got != want+1 {
				t.Errorf("subfr %d idx %d: got %d want %d", k, i, got, want)
			}
		}
	}
}

// TestAnalysisFilterPerfectPrediction — periodic signal at pitch + center
// coefficient = 1.0 should make residual ≪ input.
func TestAnalysisFilterPerfectPrediction(t *testing.T) {
	subfrLen := int32(40)
	preLen := int32(0)
	pitch := int32(20)
	pitchL := []int32{pitch, pitch, pitch, pitch}
	xStart := pitch + preLagMargin
	xLen := xStart + NBSubFr*subfrLen + preLen + 10
	x := make([]int16, xLen)
	for i := range x {
		v := math.Sin(2*math.Pi*float64(i)/float64(pitch)) * 5000
		x[i] = int16(v)
	}
	coef := make([]int16, LTPOrder*NBSubFr)
	for k := int32(0); k < NBSubFr; k++ {
		coef[k*LTPOrder+LTPOrder/2] = 1 << 14 // center tap = 1.0
	}
	gains := []int32{1 << 16, 1 << 16, 1 << 16, 1 << 16}
	res := make([]int16, NBSubFr*(subfrLen+preLen))
	AnalysisFilter(res, x, xStart, coef, pitchL, gains, subfrLen, preLen)

	var resE, inE float64
	for _, v := range res {
		resE += float64(v) * float64(v)
	}
	for i := int32(0); i < NBSubFr*subfrLen; i++ {
		v := float64(x[xStart+i])
		inE += v * v
	}
	if resE > inE*0.1 {
		t.Errorf("residual energy %v not << input energy %v (ratio %.3f)",
			resE, inE, resE/inE)
	}
}

// TestAnalysisFilterInvGainScaling — doubling invGain doubles residual.
func TestAnalysisFilterInvGainScaling(t *testing.T) {
	subfrLen := int32(20)
	preLen := int32(10)
	pitchL := []int32{15, 15, 15, 15}
	xStart := int32(15) + preLagMargin
	xLen := xStart + NBSubFr*subfrLen + preLen
	x := make([]int16, xLen)
	rng := rand.New(rand.NewSource(2))
	for i := range x {
		x[i] = int16(rng.Intn(2001) - 1000)
	}
	coef := make([]int16, LTPOrder*NBSubFr)
	for i := range coef {
		coef[i] = int16(rng.Intn(401) - 200)
	}
	gains1 := []int32{1 << 16, 1 << 16, 1 << 16, 1 << 16}
	gains2 := []int32{2 << 16, 2 << 16, 2 << 16, 2 << 16}
	r1 := make([]int16, NBSubFr*(subfrLen+preLen))
	r2 := make([]int16, NBSubFr*(subfrLen+preLen))
	AnalysisFilter(r1, x, xStart, coef, pitchL, gains1, subfrLen, preLen)
	AnalysisFilter(r2, x, xStart, coef, pitchL, gains2, subfrLen, preLen)
	for i := range r1 {
		want := int32(r1[i]) * 2
		if want > 32767 {
			want = 32767
		} else if want < -32768 {
			want = -32768
		}
		diff := int32(r2[i]) - want
		if diff < -2 || diff > 2 {
			t.Errorf("idx %d: r2=%d want ~%d (r1=%d)", i, r2[i], want, r1[i])
		}
	}
}
