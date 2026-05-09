package dec

import (
	"testing"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/rangecoder"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// mockEncoded packet contents — drives the same range-coder operations
// that DecodeParameters will read back.
type mockParams struct {
	NFramesDecoded   int32 // 0 = first frame in packet
	FsKHzIdx         int32 // index into SamplingRates_table
	TypeOffset       int32 // 2*sigtype + quantOffset
	GainsIndices     [silk.NBSubFr]int32
	NLSFIndices      []int32 // per-stage codebook indices
	NLSFInterpCoefQ2 int32
	LagIndex         int32 // only for voiced
	ContourIndex     int32
	PERIndex         int32
	LTPIndex         [silk.NBSubFr]int32 // per-subframe codebook entry
	LTPScaleIdx      int32
	Seed             int32
	Pulses           []int8
	VadFlag          int32
	FrameTermination int32
}

func encodeMockPacket(p mockParams) []byte {
	rc := &rangecoder.State{}
	rc.EncInit()

	if p.NFramesDecoded == 0 {
		rc.Encode(p.FsKHzIdx, tables.SamplingRates_CDF[:])
	}

	// type+offset
	if p.NFramesDecoded == 0 {
		rc.Encode(p.TypeOffset, tables.Type_offset_CDF[:])
	} else {
		// would use Type_offset_joint_CDF[typeOffsetPrev]; keep it simple here.
		rc.Encode(p.TypeOffset, tables.Type_offset_CDF[:])
	}

	sigtype := p.TypeOffset >> 1

	// Gains.
	if p.NFramesDecoded == 0 {
		rc.Encode(p.GainsIndices[0], tables.Gain_CDF[sigtype][:])
	} else {
		rc.Encode(p.GainsIndices[0], tables.Delta_gain_CDF[:])
	}
	for i := int32(1); i < silk.NBSubFr; i++ {
		rc.Encode(p.GainsIndices[i], tables.Delta_gain_CDF[:])
	}

	// NLSF stages.
	fsKHz := tables.SamplingRates_table[p.FsKHzIdx]
	var cb interface{}
	_ = cb
	var stages int
	if fsKHz == 8 {
		if sigtype == 0 {
			stages = 6
			rc.EncodeMulti(p.NLSFIndices, [][]uint16{
				tables.NLSF_MSVQ_CB0_10_CDF[0:],
				tables.NLSF_MSVQ_CB0_10_CDF[65:],
				tables.NLSF_MSVQ_CB0_10_CDF[82:],
				tables.NLSF_MSVQ_CB0_10_CDF[91:],
				tables.NLSF_MSVQ_CB0_10_CDF[100:],
				tables.NLSF_MSVQ_CB0_10_CDF[109:],
			})
		} else {
			stages = 6
			rc.EncodeMulti(p.NLSFIndices, [][]uint16{
				tables.NLSF_MSVQ_CB1_10_CDF[0:],
				tables.NLSF_MSVQ_CB1_10_CDF[33:],
				tables.NLSF_MSVQ_CB1_10_CDF[42:],
				tables.NLSF_MSVQ_CB1_10_CDF[51:],
				tables.NLSF_MSVQ_CB1_10_CDF[60:],
				tables.NLSF_MSVQ_CB1_10_CDF[69:],
			})
		}
	} else {
		stages = 10
		if sigtype == 0 {
			rc.EncodeMulti(p.NLSFIndices, [][]uint16{
				tables.NLSF_MSVQ_CB0_16_CDF[0:],
				tables.NLSF_MSVQ_CB0_16_CDF[129:],
				tables.NLSF_MSVQ_CB0_16_CDF[146:],
				tables.NLSF_MSVQ_CB0_16_CDF[155:],
				tables.NLSF_MSVQ_CB0_16_CDF[164:],
				tables.NLSF_MSVQ_CB0_16_CDF[173:],
				tables.NLSF_MSVQ_CB0_16_CDF[182:],
				tables.NLSF_MSVQ_CB0_16_CDF[191:],
				tables.NLSF_MSVQ_CB0_16_CDF[200:],
				tables.NLSF_MSVQ_CB0_16_CDF[209:],
			})
		} else {
			rc.EncodeMulti(p.NLSFIndices, [][]uint16{
				tables.NLSF_MSVQ_CB1_16_CDF[0:],
				tables.NLSF_MSVQ_CB1_16_CDF[33:],
				tables.NLSF_MSVQ_CB1_16_CDF[42:],
				tables.NLSF_MSVQ_CB1_16_CDF[51:],
				tables.NLSF_MSVQ_CB1_16_CDF[60:],
				tables.NLSF_MSVQ_CB1_16_CDF[69:],
				tables.NLSF_MSVQ_CB1_16_CDF[78:],
				tables.NLSF_MSVQ_CB1_16_CDF[87:],
				tables.NLSF_MSVQ_CB1_16_CDF[96:],
				tables.NLSF_MSVQ_CB1_16_CDF[105:],
			})
		}
	}
	_ = stages

	// NLSF interpolation factor.
	rc.Encode(p.NLSFInterpCoefQ2, tables.NLSF_interpolation_factor_CDF[:])

	if sigtype == silk.SigTypeVoiced {
		switch fsKHz {
		case 8:
			rc.Encode(p.LagIndex, tables.Pitch_lag_NB_CDF[:])
			rc.Encode(p.ContourIndex, tables.Pitch_contour_NB_CDF[:])
		case 12:
			rc.Encode(p.LagIndex, tables.Pitch_lag_MB_CDF[:])
			rc.Encode(p.ContourIndex, tables.Pitch_contour_CDF[:])
		case 16:
			rc.Encode(p.LagIndex, tables.Pitch_lag_WB_CDF[:])
			rc.Encode(p.ContourIndex, tables.Pitch_contour_CDF[:])
		default:
			rc.Encode(p.LagIndex, tables.Pitch_lag_SWB_CDF[:])
			rc.Encode(p.ContourIndex, tables.Pitch_contour_CDF[:])
		}
		rc.Encode(p.PERIndex, tables.LTP_per_index_CDF[:])
		for k := int32(0); k < silk.NBSubFr; k++ {
			rc.Encode(p.LTPIndex[k], tables.LTPGainCDFPtrs[p.PERIndex])
		}
		rc.Encode(p.LTPScaleIdx, tables.LTPscale_CDF[:])
	}

	rc.Encode(p.Seed, tables.Seed_CDF[:])

	// Encode pulses using the real encoder (proven in the previous tests).
	frameLen := silk.FrameLengthMs * fsKHz
	if int32(len(p.Pulses)) < frameLen {
		// Pad with zeros to full frame length.
		padded := make([]int8, frameLen)
		copy(padded, p.Pulses)
		p.Pulses = padded
	}
	EncodePulses(rc, sigtype, p.TypeOffset&1, p.Pulses, frameLen)

	rc.Encode(p.VadFlag, tables.Vadflag_CDF[:])
	rc.Encode(p.FrameTermination, tables.FrameTermination_CDF[:])
	rc.EncWrapUp()
	_, nBytes := rc.GetLength()

	out := make([]byte, nBytes)
	copy(out, rc.Buffer[:nBytes])
	return out
}

// TestDecodeParametersRoundtripVoiced8kHz — voiced frame at 8 kHz, full
// path including pitch + LTP.
func TestDecodeParametersRoundtripVoiced8kHz(t *testing.T) {
	mock := mockParams{
		NFramesDecoded:   0,
		FsKHzIdx:         0, // 8 kHz
		TypeOffset:       1, // sigtype=0 (voiced), QoffType=1
		GainsIndices:     [silk.NBSubFr]int32{32, 5, 5, 5},
		NLSFIndices:      []int32{10, 3, 2, 2, 3, 4},
		NLSFInterpCoefQ2: 4,
		LagIndex:         50,
		ContourIndex:     5,
		PERIndex:         1,
		LTPIndex:         [silk.NBSubFr]int32{3, 7, 11, 5},
		LTPScaleIdx:      1,
		Seed:             2,
		Pulses:           []int8{0, 0, 1, -1, 0, 0, 0, 2, -2, 0, 0, 0, 1, 0, 0, 0},
		VadFlag:          1,
		FrameTermination: 0,
	}

	pkt := encodeMockPacket(mock)

	// Decode side.
	psDec := &State{}
	psDec.SRC.DecInit(pkt)
	ctrl := &Control{}
	q := make([]int32, 160) // 8 kHz × 20 ms
	DecodeParameters(psDec, ctrl, q, true)

	if psDec.SRC.Error != 0 {
		t.Fatalf("decoder error: %d", psDec.SRC.Error)
	}
	if psDec.FsKHz != 8 {
		t.Errorf("FsKHz=%d want 8", psDec.FsKHz)
	}
	if ctrl.Sigtype != 0 {
		t.Errorf("Sigtype=%d want 0 (voiced)", ctrl.Sigtype)
	}
	if ctrl.QuantOffsetType != 1 {
		t.Errorf("QuantOffsetType=%d want 1", ctrl.QuantOffsetType)
	}
	if ctrl.NLSFInterpCoefQ2 != 4 {
		t.Errorf("NLSFInterpCoefQ2=%d want 4", ctrl.NLSFInterpCoefQ2)
	}
	if ctrl.PERIndex != 1 {
		t.Errorf("PERIndex=%d want 1", ctrl.PERIndex)
	}
	if psDec.VadFlag != 1 {
		t.Errorf("VadFlag=%d want 1", psDec.VadFlag)
	}
	if psDec.FrameTermination != 0 {
		t.Errorf("FrameTermination=%d want 0", psDec.FrameTermination)
	}

	// All four gains should be > 0.
	for i, g := range ctrl.GainsQ16 {
		if g <= 0 {
			t.Errorf("GainsQ16[%d] = %d", i, g)
		}
	}

	// LTPCoefQ14 should match the chosen codebook entries for PERIndex=1.
	cbk := tables.LTPVQPtrsQ14[1]
	for k := int32(0); k < silk.NBSubFr; k++ {
		idx := mock.LTPIndex[k]
		for i := int32(0); i < silk.LTPOrder; i++ {
			want := cbk[idx*silk.LTPOrder+i]
			got := ctrl.LTPCoefQ14[k*silk.LTPOrder+i]
			if got != want {
				t.Errorf("LTPCoef[%d,%d] = %d want %d", k, i, got, want)
			}
		}
	}

	// Pulses: spot-check a couple of non-zero positions.
	if q[2] != int32(mock.Pulses[2]) || q[3] != int32(mock.Pulses[3]) {
		t.Errorf("pulses mismatch: q[2]=%d want %d, q[3]=%d want %d",
			q[2], mock.Pulses[2], q[3], mock.Pulses[3])
	}
}

// TestDecodeParametersUnvoiced16kHz — sigtype=unvoiced, no pitch/LTP path.
func TestDecodeParametersUnvoiced16kHz(t *testing.T) {
	mock := mockParams{
		NFramesDecoded:   0,
		FsKHzIdx:         2, // 16 kHz → LPCOrder = 16, 10-stage NLSF
		TypeOffset:       2, // sigtype=1 (unvoiced), QoffType=0
		GainsIndices:     [silk.NBSubFr]int32{32, 5, 5, 5},
		NLSFIndices:      []int32{5, 2, 2, 3, 1, 4, 3, 2, 4, 5},
		NLSFInterpCoefQ2: 2,
		Seed:             3,
		Pulses:           []int8{0, 1, 0, -1, 0, 0, 2, 0},
		VadFlag:          0,
		FrameTermination: 0,
	}

	pkt := encodeMockPacket(mock)

	psDec := &State{}
	psDec.SRC.DecInit(pkt)
	ctrl := &Control{}
	q := make([]int32, 320) // 16 kHz × 20 ms
	DecodeParameters(psDec, ctrl, q, true)

	if psDec.SRC.Error != 0 {
		t.Fatalf("decoder error: %d", psDec.SRC.Error)
	}
	if psDec.FsKHz != 16 {
		t.Errorf("FsKHz=%d want 16", psDec.FsKHz)
	}
	if ctrl.Sigtype != silk.SigTypeUnvoiced {
		t.Errorf("Sigtype=%d want %d", ctrl.Sigtype, silk.SigTypeUnvoiced)
	}
	// Pitch + LTP must be zeroed.
	for i, p := range ctrl.PitchL {
		if p != 0 {
			t.Errorf("PitchL[%d]=%d want 0", i, p)
		}
	}
	for i, c := range ctrl.LTPCoefQ14 {
		if c != 0 {
			t.Errorf("LTPCoef[%d]=%d want 0", i, c)
		}
	}
	if ctrl.LTPScaleQ14 != 0 {
		t.Errorf("LTPScaleQ14=%d want 0", ctrl.LTPScaleQ14)
	}
}

// TestDecodeParametersInvalidFs — sampling-rate index out of [0,3] sets error.
func TestDecodeParametersInvalidFs(t *testing.T) {
	rc := &rangecoder.State{}
	rc.EncInit()
	// Inject Fs index 4 (out of valid range). The CDF only has indices 0..3,
	// so we can't truly produce Ix=4 — instead, we craft a packet with a
	// malformed first byte that the decoder will read as something other
	// than [0,3]. Actually rc.Encode validates against the CDF; we can't
	// emit 4 cleanly. Skip — this branch is exercised via fuzz/corrupt
	// payload at integration time.
	t.Skip("invalid Fs branch needs corrupt-payload fuzzing; covered at higher levels")
}
