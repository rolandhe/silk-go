package wav

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

// helper to assemble a valid mono/16-bit WAV with `dataLen` bytes of
// data. Optional `extra` bytes are appended after the data chunk so we
// can verify Open's data-chunk boundary clamp.
func makeWAV(t *testing.T, sampleRate uint32, data []byte, extra []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteHeader(&buf, sampleRate, uint32(len(data))); err != nil {
		t.Fatal(err)
	}
	buf.Write(data)
	buf.Write(extra)
	return buf.Bytes()
}

// TestWriteHeaderRoundTrip — writing then re-parsing recovers the same
// sample rate and reports the correct data byte count.
func TestWriteHeaderRoundTrip(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	raw := makeWAV(t, 16000, data, nil)

	r, err := Open(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if r.Info == nil {
		t.Fatal("Open did not detect WAV")
	}
	if r.Info.SampleRate != 16000 {
		t.Errorf("SampleRate = %d, want 16000", r.Info.SampleRate)
	}
	if r.Info.Channels != 1 || r.Info.BitsPerSample != 16 {
		t.Errorf("channels/bps mismatch: %d / %d", r.Info.Channels, r.Info.BitsPerSample)
	}
	got, err := io.ReadAll(r.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("data mismatch: got %v, want %v", got, data)
	}
}

// TestReaderStopsAtDataChunkBoundary — Open's LimitReader must not spill
// into post-data bytes (rust-silk's wav_reader_stops_at_data_chunk_boundary
// test, ported).
func TestReaderStopsAtDataChunkBoundary(t *testing.T) {
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	extra := []byte("trailing-junk-not-data") // post-data garbage
	raw := makeWAV(t, 16000, data, extra)

	r, err := Open(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("read past data chunk: got %v, want %v", got, data)
	}
}

// TestRawPCMPassThrough — Open of a non-RIFF stream returns the input
// reader with the prefix replayed.
func TestRawPCMPassThrough(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E}
	r, err := Open(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if r.Info != nil {
		t.Fatal("Open misdetected raw PCM as WAV")
	}
	got, err := io.ReadAll(r.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("data mismatch: got %v, want %v", got, data)
	}
}

// TestRejectsHugeFmtChunk — a malicious WAV claiming a 4 GB fmt chunk
// must be rejected before any allocation. Regression test.
func TestRejectsHugeFmtChunk(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(0xFFFFFFFF)) // 4 GB
	// No actual fmt body — Open should reject before reading.

	_, err := Open(bytes.NewReader(buf.Bytes()))
	if err == nil || !strings.Contains(err.Error(), "fmt chunk too large") {
		t.Errorf("expected fmt-too-large error, got %v", err)
	}
}

// TestRejectsNonPCM — non-PCM audio format must error out cleanly.
func TestRejectsNonPCM(t *testing.T) {
	// Build a header with format=3 (IEEE float).
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(3)) // float
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint32(16000))
	binary.Write(&buf, binary.LittleEndian, uint32(64000))
	binary.Write(&buf, binary.LittleEndian, uint16(4))
	binary.Write(&buf, binary.LittleEndian, uint16(32))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	_, err := Open(bytes.NewReader(buf.Bytes()))
	if err == nil || !strings.Contains(err.Error(), "PCM") {
		t.Errorf("expected PCM error, got %v", err)
	}
}
