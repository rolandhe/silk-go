package silkcli

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/rolandhe/silk-go/pkg/wav"
)

// writePCM writes int16 samples as little-endian PCM bytes to a file.
func writePCM(t *testing.T, path string, samples []int16) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		buf[2*i] = byte(uint16(s))
		buf[2*i+1] = byte(uint16(s) >> 8)
	}
	if _, err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
}

// readPCM loads a raw PCM file as int16 samples.
func readPCM(t *testing.T, path string) []int16 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(uint16(data[2*i]) | uint16(data[2*i+1])<<8)
	}
	return samples
}

// makeSineSamples — generate `nSamples` of a sine at given freq/amp.
func makeSineSamples(nSamples int, freq, sampleRate, amp float64) []int16 {
	out := make([]int16, nSamples)
	for i := range out {
		out[i] = int16(amp * math.Sin(2*math.Pi*freq*float64(i)/sampleRate))
	}
	return out
}

// TestParseEncodeArgsDefaults — minimal args fill in all defaults.
func TestParseEncodeArgsDefaults(t *testing.T) {
	a, err := ParseEncodeArgs([]string{"-i", "in.pcm", "-o", "out.silk"})
	if err != nil {
		t.Fatal(err)
	}
	if a.SampleRate != 24000 || a.Bitrate != 25000 || a.Complexity != 2 {
		t.Errorf("defaults wrong: %+v", a)
	}
	if a.PacketMs != 20 {
		t.Errorf("PacketMs = %d, want 20", a.PacketMs)
	}
}

// TestParseEncodeArgsRejectsUnknown — unknown flag → error.
func TestParseEncodeArgsRejectsUnknown(t *testing.T) {
	_, err := ParseEncodeArgs([]string{"-i", "in", "-o", "out", "--bogus"})
	if err == nil {
		t.Errorf("expected error on unknown flag")
	}
}

// TestParseDecodeArgsTolerant — tolerant=skip and tolerant=silence parse.
func TestParseDecodeArgsTolerant(t *testing.T) {
	cases := map[string]TolerantMode{
		"skip":    TolerantSkip,
		"silence": TolerantSilence,
	}
	for input, want := range cases {
		a, err := ParseDecodeArgs([]string{"-i", "in", "-o", "out", "--tolerant", input})
		if err != nil {
			t.Errorf("%s: %v", input, err)
			continue
		}
		if a.Tolerant != want {
			t.Errorf("%s: Tolerant = %d, want %d", input, a.Tolerant, want)
		}
	}
}

// TestEncodeDecodeRoundTripDefault — silk-go encode → silk-go decode round-trip
// produces non-trivial PCM that's energetically close to the input.
// Mirrors rust-silk's roundtrip_default_header test.
func TestEncodeDecodeRoundTripDefault(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.pcm")
	silkPath := filepath.Join(dir, "out.silk")
	pcmPath := filepath.Join(dir, "out.pcm")

	// 200ms at 16 kHz: 1 kHz sine, half-amplitude.
	const sampleRate = 16000
	in := makeSineSamples(sampleRate*200/1000, 1000, sampleRate, 8000)
	writePCM(t, inPath, in)

	encArgs := &EncodeArgs{
		Input:           inPath,
		Output:          silkPath,
		SampleRate:      sampleRate,
		MaxInternalRate: sampleRate,
		PacketMs:        20,
		Bitrate:         25000,
		Complexity:      0,
		Quiet:           true,
	}
	if err := Encode(encArgs); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	st, err := os.Stat(silkPath)
	if err != nil {
		t.Fatalf("output stat: %v", err)
	}
	if st.Size() < int64(len(SilkHeader)+2) {
		t.Errorf("silk output too small: %d bytes", st.Size())
	}

	decArgs := &DecodeArgs{
		Input:      silkPath,
		Output:     pcmPath,
		SampleRate: sampleRate,
		Quiet:      true,
	}
	if err := Decode(decArgs); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	out := readPCM(t, pcmPath)
	if len(out) == 0 {
		t.Fatal("decoder produced no PCM")
	}
	// Sample count tolerance: SILK adds 1 frame of look-ahead delay so
	// output ≥ input is normal.
	if len(out) < len(in)/2 {
		t.Errorf("decoder output too short: %d, input was %d", len(out), len(in))
	}

	// Energy check: decoded signal should have non-trivial energy.
	var energy float64
	for _, s := range out {
		energy += float64(s) * float64(s)
	}
	rms := math.Sqrt(energy / float64(len(out)))
	if rms < 100 {
		t.Errorf("output RMS too low: %.1f", rms)
	}
}

// TestEncodeDecodeRoundTripTencent — same round-trip with --tencent
// header prefix. Mirrors rust-silk's roundtrip_tencent_header test.
func TestEncodeDecodeRoundTripTencent(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.pcm")
	silkPath := filepath.Join(dir, "out.silk")
	pcmPath := filepath.Join(dir, "out.pcm")

	const sampleRate = 16000
	in := makeSineSamples(sampleRate*200/1000, 800, sampleRate, 6000)
	writePCM(t, inPath, in)

	if err := Encode(&EncodeArgs{
		Input:           inPath,
		Output:          silkPath,
		SampleRate:      sampleRate,
		MaxInternalRate: sampleRate,
		PacketMs:        20,
		Bitrate:         25000,
		Tencent:         true,
		Complexity:      0,
		Quiet:           true,
	}); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Verify the file starts with 0x02 + "#!SILK_V3".
	data, err := os.ReadFile(silkPath)
	if err != nil {
		t.Fatal(err)
	}
	if data[0] != TencentPrefix {
		t.Errorf("first byte = %#x, want %#x", data[0], TencentPrefix)
	}
	if string(data[1:1+len(SilkHeader)]) != SilkHeader {
		t.Errorf("missing SILK header after Tencent prefix")
	}

	if err := Decode(&DecodeArgs{
		Input:      silkPath,
		Output:     pcmPath,
		SampleRate: sampleRate,
		Quiet:      true,
	}); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	out := readPCM(t, pcmPath)
	if len(out) == 0 {
		t.Fatal("decoder produced no PCM")
	}
}

// TestEncodeDecodeWAVInput — encoder accepts a WAV file as input,
// auto-detects the sample rate. Mirrors rust-silk's roundtrip_wav_input.
func TestEncodeDecodeWAVInput(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "in.wav")
	silkPath := filepath.Join(dir, "out.silk")
	wavOutPath := filepath.Join(dir, "out.wav")

	const sampleRate = 16000
	in := makeSineSamples(sampleRate*200/1000, 1200, sampleRate, 7000)

	// Build a WAV with the int16 samples.
	var buf bytes.Buffer
	if err := wav.WriteHeader(&buf, sampleRate, uint32(len(in)*2)); err != nil {
		t.Fatal(err)
	}
	for _, s := range in {
		binary.Write(&buf, binary.LittleEndian, s)
	}
	if err := os.WriteFile(wavPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	// Encoder picks up sample rate from the WAV header — leave SampleRateSet=false.
	if err := Encode(&EncodeArgs{
		Input:           wavPath,
		Output:          silkPath,
		MaxInternalRate: sampleRate,
		PacketMs:        20,
		Bitrate:         25000,
		Complexity:      0,
		Quiet:           true,
	}); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Decode to WAV.
	if err := Decode(&DecodeArgs{
		Input:      silkPath,
		Output:     wavOutPath,
		SampleRate: sampleRate,
		WAV:        true,
		Quiet:      true,
	}); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// The output must start with a valid WAV header that points at non-zero data.
	outData, err := os.ReadFile(wavOutPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(outData) <= wav.HeaderSize {
		t.Fatalf("output WAV too small: %d bytes", len(outData))
	}
	if string(outData[0:4]) != "RIFF" || string(outData[8:12]) != "WAVE" {
		t.Errorf("output is not a valid WAV file")
	}
	dataLen := binary.LittleEndian.Uint32(outData[40:44])
	if dataLen == 0 {
		t.Errorf("WAV data chunk reports 0 bytes")
	}
	if int(dataLen) != len(outData)-wav.HeaderSize {
		t.Errorf("data length %d does not match payload %d",
			dataLen, len(outData)-wav.HeaderSize)
	}
}

// TestEncode48kHzInputUses24kInternal — feed a 48 kHz API rate; the
// encoder downsamples internally. Mirrors rust-silk's
// roundtrip_48k_input_uses_24k_internal_rate test.
func TestEncode48kHzInputUses24kInternal(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.pcm")
	silkPath := filepath.Join(dir, "out.silk")
	pcmPath := filepath.Join(dir, "out.pcm")

	const sampleRate = 48000
	in := makeSineSamples(sampleRate*200/1000, 1000, sampleRate, 7000)
	writePCM(t, inPath, in)

	if err := Encode(&EncodeArgs{
		Input:           inPath,
		Output:          silkPath,
		SampleRate:      sampleRate,
		MaxInternalRate: 24000,
		PacketMs:        20,
		Bitrate:         28000,
		Complexity:      0,
		Quiet:           true,
	}); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if err := Decode(&DecodeArgs{
		Input:      silkPath,
		Output:     pcmPath,
		SampleRate: sampleRate,
		Quiet:      true,
	}); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	out := readPCM(t, pcmPath)
	if len(out) == 0 {
		t.Fatal("decoder produced no PCM at 48 kHz API")
	}
}
