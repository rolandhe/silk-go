package silksdk_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/rolandhe/silk-go/pkg/silkcli"
	"github.com/rolandhe/silk-go/pkg/silksdk"
)

// genTone produces durationMs of a 440Hz sine + low-freq drift, scaled to
// roughly [-0.5, 0.5] full-scale, at the given sample rate. Adequate to
// exercise voiced + unvoiced encoder paths.
func genTone(sampleRate int, durationMs int) []int16 {
	n := sampleRate * durationMs / 1000
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		// Sweep the fundamental + slow amplitude modulation.
		fund := math.Sin(2 * math.Pi * (220 + 60*math.Sin(0.5*t)) * t)
		am := 0.5 + 0.3*math.Sin(2*math.Pi*4*t)
		out[i] = int16(fund * am * 12000)
	}
	return out
}

func samplesToBytes(s []int16) []byte {
	out := make([]byte, len(s)*2)
	for i, v := range s {
		out[2*i] = byte(uint16(v))
		out[2*i+1] = byte(uint16(v) >> 8)
	}
	return out
}

// TestEncodeRoundTripInMemory: encode → decode and check that the round-trip
// produces non-trivial PCM with energy comparable to the input. Mirrors the
// energy-only check from silkcli's round-trip test (SILK is lossy enough that
// per-sample SNR drifts even for clean tones, so we don't assert on it).
func TestEncodeRoundTripInMemory(t *testing.T) {
	// 200 ms 1 kHz sine at half-amplitude — same signal silkcli's round-trip
	// test uses.
	const sampleRate = 16000
	in := make([]int16, sampleRate*200/1000)
	for i := range in {
		in[i] = int16(8000 * math.Sin(2*math.Pi*1000*float64(i)/float64(sampleRate)))
	}
	pcm := samplesToBytes(in)

	var silkBuf bytes.Buffer
	if err := silksdk.Encode(silksdk.EncodeOptions{
		Input:      bytes.NewReader(pcm),
		Output:     &silkBuf,
		SampleRate: sampleRate,
		BitRate:    25000,
		Complexity: 0,
		// ComplexitySet so 0 is honored (not defaulted to 2).
		ComplexitySet: true,
	}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if silkBuf.Len() < len(silkcli.SilkHeader)+2 {
		t.Fatalf("encoded silk too small: %d", silkBuf.Len())
	}

	var pcmBuf bytes.Buffer
	if err := silksdk.Decode(silksdk.DecodeOptions{
		Input:      &silkBuf,
		Output:     &pcmBuf,
		SampleRate: sampleRate,
	}); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	out := pcmBuf.Bytes()
	if len(out) == 0 {
		t.Fatal("decoder produced no PCM")
	}
	// SILK adds ≤1 frame of look-ahead; output ≥ input/2 is fine.
	if len(out) < len(pcm)/2 {
		t.Errorf("decoder output too short: %d, input was %d", len(out), len(pcm))
	}
	// Decoded signal should have non-trivial energy.
	var energy float64
	for i := 0; i < len(out)/2; i++ {
		s := float64(int16(binary.LittleEndian.Uint16(out[2*i:])))
		energy += s * s
	}
	rms := math.Sqrt(energy / float64(len(out)/2))
	if rms < 100 {
		t.Errorf("output RMS too low: %.1f", rms)
	}
}

// TestEncodeBytesMatchSilkCLI is the critical regression: the SDK output
// must be identical to silkcli output for the same options. Since silkcli
// is verified byte-identical to rust-silk (805/805 cells), this transitively
// locks the SDK to rust-silk too.
func TestEncodeBytesMatchSilkCLI(t *testing.T) {
	pcm := samplesToBytes(genTone(16000, 500))

	type cfg struct {
		name       string
		sampleRate int
		complexity int
		bitRate    int
		fec        bool
		dtx        bool
		tencent    bool
	}
	configs := []cfg{
		{"16k_cx0_br16k", 16000, 0, 16000, false, false, false},
		{"16k_cx2_br25k", 16000, 2, 25000, false, false, false},
		{"16k_cx2_br25k_fec", 16000, 2, 25000, true, false, false},
		{"16k_cx2_br25k_dtx", 16000, 2, 25000, false, true, false},
		{"16k_cx1_br20k_fec_dtx", 16000, 1, 20000, true, true, false},
		{"16k_cx2_br25k_tencent", 16000, 2, 25000, false, false, true},
	}

	for _, c := range configs {
		t.Run(c.name, func(t *testing.T) {
			tmp := t.TempDir()
			pcmPath := filepath.Join(tmp, "in.pcm")
			cliOut := filepath.Join(tmp, "cli.silk")
			if err := os.WriteFile(pcmPath, pcm, 0o644); err != nil {
				t.Fatal(err)
			}

			cliArgs := &silkcli.EncodeArgs{
				Input:           pcmPath,
				Output:          cliOut,
				SampleRate:      int32(c.sampleRate),
				SampleRateSet:   true,
				MaxInternalRate: int32(min(c.sampleRate, 24000)),
				MaxInternalSet:  true,
				PacketMs:        20,
				Bitrate:         int32(c.bitRate),
				Complexity:      int32(c.complexity),
				Tencent:         c.tencent,
				Quiet:           true,
			}
			if c.fec {
				cliArgs.FEC = 1
			}
			if c.dtx {
				cliArgs.DTX = 1
			}
			if err := silkcli.Encode(cliArgs); err != nil {
				t.Fatalf("silkcli.Encode: %v", err)
			}

			var sdkBuf bytes.Buffer
			sdkOpts := silksdk.EncodeOptions{
				Input:           bytes.NewReader(pcm),
				Output:          &sdkBuf,
				SampleRate:      c.sampleRate,
				MaxInternalRate: min(c.sampleRate, 24000),
				PacketMs:        20,
				BitRate:         c.bitRate,
				Complexity:      c.complexity,
				ComplexitySet:   true,
				UseFEC:          c.fec,
				UseDTX:          c.dtx,
				Tencent:         c.tencent,
			}
			if err := silksdk.Encode(sdkOpts); err != nil {
				t.Fatalf("silksdk.Encode: %v", err)
			}

			cliBytes, err := os.ReadFile(cliOut)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(cliBytes, sdkBuf.Bytes()) {
				t.Errorf("cli vs sdk diverge: cli=%d bytes, sdk=%d bytes; first diff at %d",
					len(cliBytes), sdkBuf.Len(), firstDiff(cliBytes, sdkBuf.Bytes()))
			}
		})
	}
}

// TestDecodeBytesMatchSilkCLI: decoded PCM must be identical between SDK and CLI.
func TestDecodeBytesMatchSilkCLI(t *testing.T) {
	pcm := samplesToBytes(genTone(16000, 500))

	// First encode via the CLI to get a known-good .silk for both decoders.
	tmp := t.TempDir()
	pcmPath := filepath.Join(tmp, "in.pcm")
	silkPath := filepath.Join(tmp, "in.silk")
	cliPCMPath := filepath.Join(tmp, "cli_dec.pcm")
	if err := os.WriteFile(pcmPath, pcm, 0o644); err != nil {
		t.Fatal(err)
	}
	encArgs := &silkcli.EncodeArgs{
		Input: pcmPath, Output: silkPath,
		SampleRate: 16000, SampleRateSet: true,
		MaxInternalRate: 16000, MaxInternalSet: true,
		PacketMs: 20, Bitrate: 25000, Complexity: 2, Quiet: true,
	}
	if err := silkcli.Encode(encArgs); err != nil {
		t.Fatalf("encode prep: %v", err)
	}

	// Decode via CLI.
	if err := silkcli.Decode(&silkcli.DecodeArgs{
		Input: silkPath, Output: cliPCMPath,
		SampleRate: 16000, Quiet: true,
	}); err != nil {
		t.Fatalf("silkcli.Decode: %v", err)
	}
	cliBytes, err := os.ReadFile(cliPCMPath)
	if err != nil {
		t.Fatal(err)
	}

	// Decode via SDK.
	silkBytes, err := os.ReadFile(silkPath)
	if err != nil {
		t.Fatal(err)
	}
	var sdkBuf bytes.Buffer
	if err := silksdk.Decode(silksdk.DecodeOptions{
		Input: bytes.NewReader(silkBytes), Output: &sdkBuf,
		SampleRate: 16000,
	}); err != nil {
		t.Fatalf("silksdk.Decode: %v", err)
	}

	if !bytes.Equal(cliBytes, sdkBuf.Bytes()) {
		t.Errorf("decode cli vs sdk diverge: cli=%d, sdk=%d; first diff at %d",
			len(cliBytes), sdkBuf.Len(), firstDiff(cliBytes, sdkBuf.Bytes()))
	}
}

// TestWAVInputDetection: feeding a WAV (header + PCM) and the same raw PCM
// should produce the same encoded silk bytes.
func TestWAVInputDetection(t *testing.T) {
	pcm := samplesToBytes(genTone(16000, 200))

	var rawSilk bytes.Buffer
	if err := silksdk.Encode(silksdk.EncodeOptions{
		Input: bytes.NewReader(pcm), Output: &rawSilk,
		SampleRate: 16000, Complexity: 2, BitRate: 25000,
	}); err != nil {
		t.Fatal(err)
	}

	// Build a WAV containing the same PCM.
	wavBuf := makeWAV(16000, pcm)

	var wavSilk bytes.Buffer
	if err := silksdk.Encode(silksdk.EncodeOptions{
		Input: bytes.NewReader(wavBuf), Output: &wavSilk,
		// SampleRate left as 0 — should be picked up from the WAV.
		Complexity: 2, BitRate: 25000,
	}); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(rawSilk.Bytes(), wavSilk.Bytes()) {
		t.Errorf("PCM-input vs WAV-input encoded silk differ: %d vs %d bytes",
			rawSilk.Len(), wavSilk.Len())
	}
}

// TestTencentMode: Tencent stream should start with 0x02 and not end with
// 0xFF 0xFF; round-trip must work.
func TestTencentMode(t *testing.T) {
	pcm := samplesToBytes(genTone(16000, 100))

	var silkBuf bytes.Buffer
	if err := silksdk.Encode(silksdk.EncodeOptions{
		Input: bytes.NewReader(pcm), Output: &silkBuf,
		SampleRate: 16000, Complexity: 2, BitRate: 25000, Tencent: true,
	}); err != nil {
		t.Fatal(err)
	}
	b := silkBuf.Bytes()
	if len(b) < 1 || b[0] != 0x02 {
		t.Errorf("tencent stream missing 0x02 prefix; head=%x", b[:min(len(b), 16)])
	}
	if len(b) >= 2 && b[len(b)-2] == 0xFF && b[len(b)-1] == 0xFF {
		t.Errorf("tencent stream should not end with 0xFF 0xFF")
	}

	var pcmBuf bytes.Buffer
	if err := silksdk.Decode(silksdk.DecodeOptions{
		Input: bytes.NewReader(b), Output: &pcmBuf,
		SampleRate: 16000,
	}); err != nil {
		t.Fatalf("decode tencent: %v", err)
	}
	if pcmBuf.Len() == 0 {
		t.Errorf("tencent decode produced no PCM")
	}
}

// TestNonSeekableWAVOutput: WriteWAV to a *bytes.Buffer (not seekable)
// produces a valid WAV.
func TestNonSeekableWAVOutput(t *testing.T) {
	pcm := samplesToBytes(genTone(16000, 200))

	var silkBuf bytes.Buffer
	if err := silksdk.Encode(silksdk.EncodeOptions{
		Input: bytes.NewReader(pcm), Output: &silkBuf,
		SampleRate: 16000, Complexity: 2, BitRate: 25000,
	}); err != nil {
		t.Fatal(err)
	}

	var wavOut bytes.Buffer
	if err := silksdk.Decode(silksdk.DecodeOptions{
		Input: &silkBuf, Output: &wavOut,
		SampleRate: 16000, WriteWAV: true,
	}); err != nil {
		t.Fatal(err)
	}
	out := wavOut.Bytes()
	if len(out) < 44 {
		t.Fatalf("wav output too short: %d bytes", len(out))
	}
	if string(out[0:4]) != "RIFF" || string(out[8:12]) != "WAVE" {
		t.Errorf("wav header malformed: %x", out[:12])
	}
	dataLen := binary.LittleEndian.Uint32(out[40:44])
	if int(dataLen)+44 != len(out) {
		t.Errorf("wav data length mismatch: header=%d body=%d", dataLen, len(out)-44)
	}
}

// TestStatsAndMetrics: pointer fields are populated when supplied.
func TestStatsAndMetrics(t *testing.T) {
	pcm := samplesToBytes(genTone(16000, 300))

	var silkBuf bytes.Buffer
	encStats := &silksdk.EncodeStats{}
	if err := silksdk.Encode(silksdk.EncodeOptions{
		Input: bytes.NewReader(pcm), Output: &silkBuf,
		SampleRate: 16000, Complexity: 2, BitRate: 25000, Stats: encStats,
	}); err != nil {
		t.Fatal(err)
	}
	if encStats.PacketsOut == 0 {
		t.Errorf("expected PacketsOut > 0")
	}
	if encStats.BytesOut != int64(silkBuf.Len()) {
		t.Errorf("BytesOut=%d but buffer=%d", encStats.BytesOut, silkBuf.Len())
	}
	if encStats.SamplesIn == 0 {
		t.Errorf("expected SamplesIn > 0")
	}
	if encStats.DurationMs == 0 {
		t.Errorf("expected DurationMs > 0")
	}

	decMetrics := &silksdk.DecodeMetrics{}
	var pcmOut bytes.Buffer
	if err := silksdk.Decode(silksdk.DecodeOptions{
		Input: &silkBuf, Output: &pcmOut,
		SampleRate: 16000, Metrics: decMetrics,
	}); err != nil {
		t.Fatal(err)
	}
	if decMetrics.SamplesOut == 0 {
		t.Errorf("expected SamplesOut > 0")
	}
	if decMetrics.PacketsIn != encStats.PacketsOut {
		t.Errorf("PacketsIn (%d) != PacketsOut (%d)", decMetrics.PacketsIn, encStats.PacketsOut)
	}
}

// TestRejectsWAVAsSilkInput: passing a WAV-shaped stream into Decode must error.
func TestRejectsWAVAsSilkInput(t *testing.T) {
	wavBuf := makeWAV(16000, samplesToBytes(genTone(16000, 50)))
	err := silksdk.Decode(silksdk.DecodeOptions{
		Input:      bytes.NewReader(wavBuf),
		Output:     new(bytes.Buffer),
		SampleRate: 16000,
	})
	if err == nil {
		t.Fatalf("expected error decoding WAV-shaped input")
	}
}

// TestValidationErrors covers the most common option mistakes.
func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		opts silksdk.EncodeOptions
	}{
		{"missing input", silksdk.EncodeOptions{Output: new(bytes.Buffer), SampleRate: 16000}},
		{"missing output", silksdk.EncodeOptions{Input: bytes.NewReader(nil), SampleRate: 16000}},
		{"missing sample rate", silksdk.EncodeOptions{Input: bytes.NewReader([]byte{0, 0}), Output: new(bytes.Buffer)}},
		{"bad sample rate", silksdk.EncodeOptions{Input: bytes.NewReader([]byte{0, 0}), Output: new(bytes.Buffer), SampleRate: 13000}},
		{"bad complexity", silksdk.EncodeOptions{Input: bytes.NewReader([]byte{0, 0}), Output: new(bytes.Buffer), SampleRate: 16000, Complexity: 5, ComplexitySet: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := silksdk.Encode(c.opts); err == nil {
				t.Errorf("expected validation error for %q", c.name)
			}
		})
	}
}

// makeWAV wraps mono s16le PCM bytes in a 44-byte WAV header.
func makeWAV(sampleRate int, pcm []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2)) // byte rate
	binary.Write(&buf, binary.LittleEndian, uint16(2))            // block align
	binary.Write(&buf, binary.LittleEndian, uint16(16))           // bits/sample
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
