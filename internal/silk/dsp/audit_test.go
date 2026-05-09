package dsp

// audit_test.go — direct oracle verification of dsp helpers that have
// 1:1 C macro/inline equivalents (lin2log, log2lin, sigm_Q15).
//
// We don't drive the C oracle from this file (the harness lives in
// internal/silk/fix/audit_oracle_test.go); instead these tests use
// hand-checked golden values pulled from the oracle once. If you change
// the underlying algorithm you must regenerate these (run
// `tools/macro-oracle/oracle` interactively).

import (
	"testing"
)

// goldenLin2Log: pairs of (input, C-expected-output) verified via the oracle.
var goldenLin2Log = [][2]int32{
	{0, -128},
	{1, 0},
	{1024, 1280},
	{2147483647, 3967},
	{-1, 4095}, // negative input feeds CLZ_FRAC of a negative; matches C bit-pattern.
}

func TestLin2LogMatchesC(t *testing.T) {
	for _, c := range goldenLin2Log {
		if got := Lin2Log(c[0]); got != c[1] {
			t.Errorf("Lin2Log(%d) = %d, want %d (C oracle)", c[0], got, c[1])
		}
	}
}

// goldenLog2Lin: oracle pairs.
var goldenLog2Lin = [][2]int32{
	{0, 1},
	{128, 2},
	{1024, 256},
	{3967, 2130706432},
}

func TestLog2LinMatchesC(t *testing.T) {
	for _, c := range goldenLog2Lin {
		if got := Log2Lin(c[0]); got != c[1] {
			t.Errorf("Log2Lin(%d) = %d, want %d (C oracle)", c[0], got, c[1])
		}
	}
}

// goldenSigm: oracle pairs.
var goldenSigmQ15 = [][2]int32{
	{0, 16384},
	{100, 31333},
	{-100, 1434},
	{192, 32767}, // saturates at 6*32 = 192
	{-192, 0},    // saturates negative
}

func TestSigmQ15MatchesC(t *testing.T) {
	for _, c := range goldenSigmQ15 {
		if got := SigmQ15(c[0]); got != c[1] {
			t.Errorf("SigmQ15(%d) = %d, want %d (C oracle)", c[0], got, c[1])
		}
	}
}
