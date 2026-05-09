package silkcli

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/rolandhe/silk-go/pkg/wav"
)

// outputSink is the decode side's writer. It optionally wraps the raw
// PCM stream with a WAV header and tracks per-stream stats (energy,
// SNR-vs-reference, packet/frame counts).
type outputSink struct {
	target     io.Writer
	file       *os.File // non-nil when output is a regular file (needed for WAV finalize)
	wav        *wavState
	stats      *decodeStats
	sampleRate int32
}

type wavState struct {
	dataLen    uint32
	sampleRate uint32
}

type decodeStats struct {
	Packets     uint64
	Frames      uint64
	Samples     uint64
	Bytes       uint64
	Energy      float64
	RefEnergy   float64
	NoiseEnergy float64
	RefPos      int
	Reference   []int16
}

func newOutputSink(path string, sampleRate int32, forceWAV, withStats bool, reference []int16) (*outputSink, error) {
	isStdout := path == "-"
	isWAV := forceWAV || hasWAVExtension(path)
	if isStdout && isWAV {
		return nil, errors.New("cannot write WAV to stdout")
	}

	s := &outputSink{sampleRate: sampleRate}

	if isStdout {
		s.target = os.Stdout
	} else {
		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("open output: %w", err)
		}
		s.target = f
		s.file = f
	}

	if isWAV {
		s.wav = &wavState{sampleRate: uint32(sampleRate)}
		if err := wav.WriteHeader(s.target, s.wav.sampleRate, 0); err != nil {
			return nil, fmt.Errorf("write output: %w", err)
		}
	}

	if withStats {
		s.stats = &decodeStats{Reference: reference}
	}
	return s, nil
}

// WriteSamples appends `len` int16 PCM samples (little-endian) to the
// output. Updates stats counters when enabled.
func (s *outputSink) WriteSamples(buf []int16, length int) error {
	if length == 0 {
		return nil
	}
	bytes := make([]byte, length*2)
	for i := 0; i < length; i++ {
		bytes[2*i] = byte(uint16(buf[i]))
		bytes[2*i+1] = byte(uint16(buf[i]) >> 8)
	}
	if s.stats != nil {
		for i := 0; i < length; i++ {
			sample := float64(buf[i])
			s.stats.Energy += sample * sample
			s.stats.Samples++
			if s.stats.Reference != nil && s.stats.RefPos < len(s.stats.Reference) {
				r := float64(s.stats.Reference[s.stats.RefPos])
				s.stats.RefEnergy += r * r
				diff := r - sample
				s.stats.NoiseEnergy += diff * diff
				s.stats.RefPos++
			}
		}
	}
	if _, err := s.target.Write(bytes); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if s.wav != nil {
		s.wav.dataLen += uint32(length * 2)
	}
	return nil
}

// WriteSilence writes `nSamples` zero samples and updates stats as if
// they were real samples (so SNR-vs-reference doesn't drift on dropped
// packets).
func (s *outputSink) WriteSilence(nSamples int) error {
	if nSamples == 0 {
		return nil
	}
	bytes := make([]byte, nSamples*2)
	if _, err := s.target.Write(bytes); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if s.wav != nil {
		s.wav.dataLen += uint32(nSamples * 2)
	}
	if s.stats != nil {
		s.stats.Samples += uint64(nSamples)
		if s.stats.Reference != nil {
			for i := 0; i < nSamples; i++ {
				if s.stats.RefPos < len(s.stats.Reference) {
					r := float64(s.stats.Reference[s.stats.RefPos])
					s.stats.RefEnergy += r * r
					s.stats.NoiseEnergy += r * r
					s.stats.RefPos++
				}
			}
		}
	}
	return nil
}

func (s *outputSink) RecordFrames(n uint64) {
	if s.stats != nil {
		s.stats.Frames += n
	}
}

// handleDecodeFailure handles a per-frame decode error (called from the
// PLC frame loop). Returns (continue, err): continue=true means the
// caller should keep looping; err is non-nil only when the silence-write
// itself failed (still recoverable from the caller's perspective if
// continue=false).
func (s *outputSink) handleDecodeFailure(mode TolerantMode, framesLeft int32, frameSamples int32) (bool, error) {
	switch mode {
	case TolerantSkip:
		return true, nil
	case TolerantSilence:
		if framesLeft > 0 {
			samples := int(framesLeft) * int(frameSamples)
			if err := s.WriteSilence(samples); err != nil {
				return false, err
			}
			s.RecordFrames(uint64(framesLeft))
		}
		return true, nil
	default:
		return false, nil
	}
}

// handlePacketFailure is the in-packet variant — it covers a single
// frame's worth of silence and returns (continue, err) like
// handleDecodeFailure.
func (s *outputSink) handlePacketFailure(mode TolerantMode, frameSamples int32) (bool, error) {
	switch mode {
	case TolerantSkip:
		return true, nil
	case TolerantSilence:
		if err := s.WriteSilence(int(frameSamples)); err != nil {
			return false, err
		}
		s.RecordFrames(1)
		return true, nil
	default:
		return false, nil
	}
}

// Finalize seeks back to the start of the file and rewrites the WAV
// header with the final data length. No-op when output isn't WAV or the
// underlying writer doesn't support seeking.
func (s *outputSink) Finalize() error {
	if s.wav == nil || s.file == nil {
		return nil
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek output: %w", err)
	}
	if err := wav.WriteHeader(s.file, s.wav.sampleRate, s.wav.dataLen); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// Close flushes the underlying file (if any). Stdout is left alone.
func (s *outputSink) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// PrintStats — mirrors rust-silk's `metrics: ...` lines for `--metrics`
// and (when reference is provided) `--reference`.
func (s *outputSink) PrintStats(sampleRate int32) {
	if s.stats == nil {
		return
	}
	duration := float64(s.stats.Samples) / float64(sampleRate)
	var bitrate float64
	if duration > 0 {
		bitrate = float64(s.stats.Bytes) * 8 / duration
	}
	var rms float64
	if s.stats.Samples > 0 {
		rms = math.Sqrt(s.stats.Energy / float64(s.stats.Samples))
	}
	var avgFrames float64
	if s.stats.Packets > 0 {
		avgFrames = float64(s.stats.Frames) / float64(s.stats.Packets)
	}
	fmt.Fprintf(os.Stderr,
		"metrics: packets=%d frames=%d avg_frames=%.2f samples=%d duration=%.3fs bytes=%d bitrate=%.1fbps rms=%.2f\n",
		s.stats.Packets, s.stats.Frames, avgFrames, s.stats.Samples,
		duration, s.stats.Bytes, bitrate, rms)
	if s.stats.RefEnergy > 0 {
		energyRatio := s.stats.Energy / s.stats.RefEnergy
		var snr float64
		if s.stats.NoiseEnergy > 0 {
			snr = 10 * math.Log10(s.stats.RefEnergy/s.stats.NoiseEnergy)
		} else {
			snr = math.Inf(1)
		}
		fmt.Fprintf(os.Stderr,
			"metrics: ref_samples=%d energy_ratio=%.4f snr_db=%.2f\n",
			s.stats.RefPos, energyRatio, snr)
	}
}

// loadReferenceSamples reads a PCM/WAV reference file fully into an
// int16 slice. Used by --reference.
func loadReferenceSamples(path string, sampleRate int32) ([]int16, error) {
	src, closer, err := openInput(path)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	if src.Info != nil {
		if src.Info.Channels != 1 || src.Info.BitsPerSample != 16 {
			return nil, errors.New("reference wav must be mono 16-bit")
		}
		if src.Info.SampleRate != sampleRate {
			return nil, fmt.Errorf("reference sample-rate mismatch: %d vs %d",
				src.Info.SampleRate, sampleRate)
		}
	}
	bytes, err := io.ReadAll(src.Reader)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if len(bytes)%2 != 0 {
		bytes = bytes[:len(bytes)-1]
	}
	samples := make([]int16, len(bytes)/2)
	for i := range samples {
		samples[i] = int16(uint16(bytes[2*i]) | uint16(bytes[2*i+1])<<8)
	}
	return samples, nil
}
