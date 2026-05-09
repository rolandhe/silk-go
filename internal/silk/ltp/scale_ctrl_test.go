package ltp

import (
	"testing"

	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// TestScaleCtrlDefault — first call with zero gains and no payload should
// pick scale index 0 with the table's [0] entry as Q14.
func TestScaleCtrlDefault(t *testing.T) {
	s := &ScaleCtrlState{
		LTPredCodGainQ7:     0,
		PacketLossPerc:      0,
		NFramesInPayloadBuf: 0,
		PacketSizeMs:        20,
	}
	ScaleCtrl(s)
	if s.LTPScaleIndex != 0 {
		t.Errorf("LTPScaleIndex=%d want 0", s.LTPScaleIndex)
	}
	if s.LTPScaleQ14 != int32(tables.LTPScales_table_Q14[0]) {
		t.Errorf("LTPScaleQ14=%d want %d", s.LTPScaleQ14, tables.LTPScales_table_Q14[0])
	}
}

// TestScaleCtrlHighGainSelectsMax — a large LTPredCodGainQ7 drives gLimit
// past the highest threshold, picking index 2.
func TestScaleCtrlHighGainSelectsMax(t *testing.T) {
	s := &ScaleCtrlState{
		LTPredCodGainQ7:     1 << 12, // very high gain (= 32 in Q7)
		PacketLossPerc:      0,
		NFramesInPayloadBuf: 0,
		PacketSizeMs:        20,
	}
	ScaleCtrl(s)
	if s.LTPScaleIndex != 2 {
		t.Errorf("with high gain, LTPScaleIndex=%d want 2", s.LTPScaleIndex)
	}
	if s.LTPScaleQ14 != int32(tables.LTPScales_table_Q14[2]) {
		t.Errorf("LTPScaleQ14=%d want %d", s.LTPScaleQ14, tables.LTPScales_table_Q14[2])
	}
}

// TestScaleCtrlMidPayloadKeepsZero — when nFramesInPayloadBuf > 0 the
// function never re-evaluates: scale stays at the default 0 regardless
// of input gain.
func TestScaleCtrlMidPayloadKeepsZero(t *testing.T) {
	s := &ScaleCtrlState{
		LTPredCodGainQ7:     1 << 12,
		PacketLossPerc:      0,
		NFramesInPayloadBuf: 1,
		PacketSizeMs:        20,
	}
	ScaleCtrl(s)
	if s.LTPScaleIndex != 0 {
		t.Errorf("mid-payload LTPScaleIndex=%d want 0", s.LTPScaleIndex)
	}
}

// TestScaleCtrlHighLossBiasesUp — the threshold table walks DOWN with
// index, so higher PacketLossPerc → higher table index → lower threshold
// value → scaling triggers MORE easily. This is intentional: under losses,
// LTP scaling helps maintain quality.
func TestScaleCtrlHighLossBiasesUp(t *testing.T) {
	low := &ScaleCtrlState{
		LTPredCodGainQ7:     2 << 7,
		PacketLossPerc:      0,
		NFramesInPayloadBuf: 0,
		PacketSizeMs:        20,
	}
	ScaleCtrl(low)
	high := &ScaleCtrlState{
		LTPredCodGainQ7:     2 << 7,
		PacketLossPerc:      50,
		NFramesInPayloadBuf: 0,
		PacketSizeMs:        20,
	}
	ScaleCtrl(high)
	if high.LTPScaleIndex < low.LTPScaleIndex {
		t.Errorf("high-loss index=%d < low-loss index=%d (expected ≥)",
			high.LTPScaleIndex, low.LTPScaleIndex)
	}
}

// TestScaleCtrlHPFilterEvolves — repeated calls with constant input gain
// drive HPLTPredCodGain toward zero (the high-pass filters out the DC).
func TestScaleCtrlHPFilterEvolves(t *testing.T) {
	s := &ScaleCtrlState{
		LTPredCodGainQ7:     1000,
		HPLTPredCodGainQ7:   1000,
		PrevLTPredCodGainQ7: 1000,
		PacketSizeMs:        20,
	}
	for i := 0; i < 5; i++ {
		ScaleCtrl(s)
	}
	if s.HPLTPredCodGainQ7 > 250 {
		t.Errorf("HPLTPredCodGainQ7 didn't decay: %d", s.HPLTPredCodGainQ7)
	}
}

// TestScaleCtrlZeroPacketSize — defensive: zero packet size shouldn't OOB
// the threshold LUT.
func TestScaleCtrlZeroPacketSize(t *testing.T) {
	s := &ScaleCtrlState{
		LTPredCodGainQ7:     1000,
		PacketSizeMs:        0,
		PacketLossPerc:      0,
		NFramesInPayloadBuf: 0,
	}
	ScaleCtrl(s) // Must not panic.
	if s.LTPScaleIndex < 0 || s.LTPScaleIndex > 2 {
		t.Errorf("LTPScaleIndex=%d out of range", s.LTPScaleIndex)
	}
}
