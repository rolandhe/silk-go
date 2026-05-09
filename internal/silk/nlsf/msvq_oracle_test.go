package nlsf

import (
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
)

// TestMSVQEncodeMatchesCOracle — fixed Go-side regression that the MSVQ
// encoder produces a C-bit-exact path for the frame-0 input we
// extracted from voice.pcm during the round-2 audit.
//
// The expected output [31 15 5 1 3 0 2 4 3 9] was confirmed by feeding
// the same NLSF/wQ6/prev arrays through the C oracle (see
// tools/macro-oracle/oracle.c, MSVQ_ENCODE op).
//
// If this test ever fails after a change to MSVQEncode or any of its
// helpers (VQRateDistortion, VQSumError, InsertionSortIncreasing,
// MSVQDecode→Stabilize), regenerate the expected by running:
//
//	cd tools/macro-oracle && make
//	echo "MSVQ_ENCODE 2 2 16 1 33 3290 <nlsf 16 ints> <wQ6 16 ints> <prev 16 zeros>" | ./oracle
func TestMSVQEncodeMatchesCOracle(t *testing.T) {
	nlsf := []int32{2446, 2906, 4185, 4796, 7958, 9849, 10178, 11759,
		14854, 17771, 19906, 22061, 24077, 26348, 28825, 30806}
	wQ6 := []int32{5416, 6198, 5071, 4095, 1772, 7483, 7700, 2003,
		1395, 1700, 1955, 2013, 1963, 1769, 1904, 2126}
	prev := make([]int32, 16) // first frame: prev all zero
	want := []int32{31, 15, 5, 1, 3, 0, 2, 4, 3, 9}

	indices := make([]int32, 10)
	in := append([]int32(nil), nlsf...) // MSVQEncode mutates input
	MSVQEncode(indices, in, &CB0_16, prev, wQ6,
		33,               // muQ15 (voiced first frame: SmlaWB(66, -8388, ~200) ≈ 33)
		3290,             // muFlucRedQ16 (matches first-frame voiced computation)
		2,                // survivors (low complexity)
		silk.MaxLPCOrder, // D=16
		1,                // deactivateFlucRed=1 (FirstFrameAfterReset)
	)
	for i, w := range want {
		if indices[i] != w {
			t.Errorf("MSVQEncode index mismatch:\n  want %v\n  got  %v", want, indices)
			break
		}
	}
}
