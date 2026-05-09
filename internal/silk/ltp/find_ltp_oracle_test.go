package ltp

import (
	"bufio"
	"os"
	"strconv"
	"testing"
)

// TestFindLTPMatchesCOracle — regression locking Go's FindLTP against
// the C reference for the exact frame-0 input captured from voice.pcm.
//
// Inputs at testdata/findltp_voice_frame0.txt; expected output computed
// by tools/macro-oracle/oracle (FIND_LTP op) with rust-silk's vendored
// SKP_Silk_find_LTP_FIX.
func TestFindLTPMatchesCOracle(t *testing.T) {
	f, err := os.Open("testdata/findltp_voice_frame0.txt")
	if err != nil {
		t.Skipf("test data not available: %v", err)
		return
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 1<<20)
	scan.Split(bufio.ScanWords)

	next := func() string { scan.Scan(); return scan.Text() }
	nextInt := func() int32 { v, _ := strconv.Atoi(next()); return int32(v) }

	if h := next(); h != "FIND_LTP" {
		t.Fatalf("bad header: %s", h)
	}
	subfrLength := nextInt()
	memOffset := nextInt()

	lag := []int32{nextInt(), nextInt(), nextInt(), nextInt()}
	wghtQ15 := []int32{nextInt(), nextInt(), nextInt(), nextInt()}

	rFirstLen := nextInt()
	rFirst := make([]int16, rFirstLen)
	for i := range rFirst {
		rFirst[i] = int16(nextInt())
	}
	rLastLen := nextInt()
	rLast := make([]int16, rLastLen)
	for i := range rLast {
		rLast[i] = int16(nextInt())
	}

	bQ14 := make([]int16, NBSubFr*LTPOrder)
	WLTP := make([]int32, NBSubFr*LTPOrder*LTPOrder)
	corrRshifts := make([]int32, NBSubFr)
	var ltpredCodGainQ7 int32

	FindLTP(bQ14, WLTP, &ltpredCodGainQ7, rFirst, rLast, lag, wghtQ15,
		subfrLength, memOffset, corrRshifts)

	// C oracle produced (verified by running tools/macro-oracle/oracle):
	wantBQ14 := []int16{
		1, 1, 1, 1, 1,
		322, 322, 322, 322, 322,
		-781, 292, 4138, 1504, 3113,
		-1681, 3475, 7412, 3340, 212,
	}
	wantGainQ7 := int32(123)

	for i, want := range wantBQ14 {
		if bQ14[i] != want {
			t.Errorf("bQ14[%d] = %d, want %d (C oracle)", i, bQ14[i], want)
		}
	}
	if ltpredCodGainQ7 != wantGainQ7 {
		t.Errorf("ltpredCodGainQ7 = %d, want %d (C oracle)", ltpredCodGainQ7, wantGainQ7)
	}
}
