package dec

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// combinePulses — combine_pulses (file-static helper in shell_coder.c).
// out[k] = in[2k] + in[2k+1]; len = output samples.
func combinePulses(out, in []int32, length int32) {
	for k := int32(0); k < length; k++ {
		out[k] = in[2*k] + in[2*k+1]
	}
}

// encodeSplit — encode_split helper (used only by ShellEncoder).
func encodeSplit(rc *rangecoder.State, pChild1, p int32, shellTable []uint16) {
	if p > 0 {
		off := tables.Shell_code_table_offsets[p]
		rc.Encode(pChild1, shellTable[off:])
	}
}

// decodeSplit — decode_split helper (used only by ShellDecoder).
func decodeSplit(pChild1, pChild2 *int32, rc *rangecoder.State, p int32, shellTable []uint16) {
	if p > 0 {
		cdfMiddle := fix.RShift32(p, 1)
		off := tables.Shell_code_table_offsets[p]
		*pChild1 = rc.Decode(shellTable[off:], cdfMiddle)
		*pChild2 = p - *pChild1
	} else {
		*pChild1 = 0
		*pChild2 = 0
	}
}

// ShellEncoder — SKP_Silk_shell_encoder. Encodes one 16-pulse shell frame.
//
// Translated from vendor/silk/src/SKP_Silk_shell_coder.c.
func ShellEncoder(rc *rangecoder.State, pulses0 []int32) {
	var pulses1 [8]int32
	var pulses2 [4]int32
	var pulses3 [2]int32
	var pulses4 [1]int32

	combinePulses(pulses1[:], pulses0, 8)
	combinePulses(pulses2[:], pulses1[:], 4)
	combinePulses(pulses3[:], pulses2[:], 2)
	combinePulses(pulses4[:], pulses3[:], 1)

	t0 := tables.Shell_code_table0[:]
	t1 := tables.Shell_code_table1[:]
	t2 := tables.Shell_code_table2[:]
	t3 := tables.Shell_code_table3[:]

	encodeSplit(rc, pulses3[0], pulses4[0], t3)

	encodeSplit(rc, pulses2[0], pulses3[0], t2)

	encodeSplit(rc, pulses1[0], pulses2[0], t1)
	encodeSplit(rc, pulses0[0], pulses1[0], t0)
	encodeSplit(rc, pulses0[2], pulses1[1], t0)

	encodeSplit(rc, pulses1[2], pulses2[1], t1)
	encodeSplit(rc, pulses0[4], pulses1[2], t0)
	encodeSplit(rc, pulses0[6], pulses1[3], t0)

	encodeSplit(rc, pulses2[2], pulses3[1], t2)

	encodeSplit(rc, pulses1[4], pulses2[2], t1)
	encodeSplit(rc, pulses0[8], pulses1[4], t0)
	encodeSplit(rc, pulses0[10], pulses1[5], t0)

	encodeSplit(rc, pulses1[6], pulses2[3], t1)
	encodeSplit(rc, pulses0[12], pulses1[6], t0)
	encodeSplit(rc, pulses0[14], pulses1[7], t0)
}

// ShellDecoder — SKP_Silk_shell_decoder. Reconstructs one 16-pulse shell
// frame given the total pulse count for the frame.
//
// Translated from vendor/silk/src/SKP_Silk_shell_coder.c.
func ShellDecoder(pulses0 []int32, rc *rangecoder.State, pulses4 int32) {
	var pulses3 [2]int32
	var pulses2 [4]int32
	var pulses1 [8]int32

	t0 := tables.Shell_code_table0[:]
	t1 := tables.Shell_code_table1[:]
	t2 := tables.Shell_code_table2[:]
	t3 := tables.Shell_code_table3[:]

	decodeSplit(&pulses3[0], &pulses3[1], rc, pulses4, t3)

	decodeSplit(&pulses2[0], &pulses2[1], rc, pulses3[0], t2)

	decodeSplit(&pulses1[0], &pulses1[1], rc, pulses2[0], t1)
	decodeSplit(&pulses0[0], &pulses0[1], rc, pulses1[0], t0)
	decodeSplit(&pulses0[2], &pulses0[3], rc, pulses1[1], t0)

	decodeSplit(&pulses1[2], &pulses1[3], rc, pulses2[1], t1)
	decodeSplit(&pulses0[4], &pulses0[5], rc, pulses1[2], t0)
	decodeSplit(&pulses0[6], &pulses0[7], rc, pulses1[3], t0)

	decodeSplit(&pulses2[2], &pulses2[3], rc, pulses3[1], t2)

	decodeSplit(&pulses1[4], &pulses1[5], rc, pulses2[2], t1)
	decodeSplit(&pulses0[8], &pulses0[9], rc, pulses1[4], t0)
	decodeSplit(&pulses0[10], &pulses0[11], rc, pulses1[5], t0)

	decodeSplit(&pulses1[6], &pulses1[7], rc, pulses2[3], t1)
	decodeSplit(&pulses0[12], &pulses0[13], rc, pulses1[6], t0)
	decodeSplit(&pulses0[14], &pulses0[15], rc, pulses1[7], t0)
}
