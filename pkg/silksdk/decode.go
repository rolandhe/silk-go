package silksdk

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dec"
	"github.com/rolandhe/silk-go/pkg/wav"
)

// Decode reads a SILK byte stream from opts.Input and writes PCM samples
// (mono signed 16-bit little-endian) to opts.Output. The output bytes are
// byte-identical to running `silk-go decode` (and rust-silk's CLI) with
// equivalent flags.
//
// Decode does not close opts.Output. The caller owns flush/close.
//
// When opts.WriteWAV is true the SDK emits a 44-byte WAV header before
// the PCM. Because the final sample count is unknown until decoding ends,
// PCM is buffered in memory and written all at once at the end. Don't use
// WriteWAV on multi-gigabyte streams.
//
// When opts.Reference is non-nil and opts.Metrics is non-nil, Decode
// computes SNR vs the reference and populates Metrics.SNRDb /
// Metrics.EnergyRatio.
func Decode(opts DecodeOptions) error {
	r, err := opts.resolveDecode()
	if err != nil {
		return err
	}

	// Reject WAV-shaped silk input early — the WAV detector consumes the
	// first 12 bytes; we want clean .silk only.
	src, err := wav.Open(opts.Input)
	if err != nil {
		return fmt.Errorf("silksdk: read input: %w", err)
	}
	if src.Info != nil {
		return ErrUnexpectedWAVInput
	}

	// Optional reference for SNR.
	var refSamples []int16
	if opts.Reference != nil {
		refSamples, err = readReferencePCM(opts.Reference, r.sampleRate)
		if err != nil {
			return fmt.Errorf("silksdk: read reference: %w", err)
		}
	}

	// Compute SNR/stats?
	wantStats := opts.Metrics != nil || refSamples != nil

	// Output sink: when WriteWAV is true, buffer PCM in memory; we'll
	// emit header + body in one go at Finalize. Otherwise stream straight
	// to opts.Output.
	sink := newSink(opts.Output, r.writeWAV, r.sampleRate, wantStats, refSamples)

	// SILK header detect (auto-detect 0x02 prefix → strip → expect SILK_V3).
	var first [1]byte
	if _, rerr := io.ReadFull(src.Reader, first[:]); rerr != nil {
		return ErrInvalidSilkHeader
	}
	if first[0] == tencentPrefix {
		hdr := make([]byte, len(silkHeader))
		if _, rerr := io.ReadFull(src.Reader, hdr); rerr != nil {
			return ErrInvalidSilkHeader
		}
		if string(hdr) != silkHeader {
			return ErrInvalidSilkHeader
		}
	} else {
		hdr := make([]byte, len(silkHeader)-1)
		if _, rerr := io.ReadFull(src.Reader, hdr); rerr != nil {
			return ErrInvalidSilkHeader
		}
		if string(hdr) != silkHeader[1:] {
			return ErrInvalidSilkHeader
		}
	}

	var decState dec.State
	dec.InitDecoder(&decState)

	decControl := silk.DecControl{
		APISampleRate:   r.sampleRate,
		FramesPerPacket: 1,
	}

	const maxFrameSamples = silk.MaxAPIFsKHz * frameLengthMs
	out := make([]int16, maxFrameSamples)
	payload := make([]byte, maxBytesPerFrame*maxInputFrames)
	frameSamples := (frameLengthMs * r.sampleRate) / 1000
	packets := 0
	var bytesIn int64
	bytesIn = int64(len(silkHeader))
	if first[0] == tencentPrefix {
		bytesIn++
	}

	for {
		var sizeBuf [2]byte
		n, rerr := io.ReadFull(src.Reader, sizeBuf[:])
		if rerr == io.EOF || (rerr == io.ErrUnexpectedEOF && n == 0) {
			break
		}
		if rerr == io.ErrUnexpectedEOF {
			return ErrTruncatedPacket
		}
		if rerr != nil {
			return fmt.Errorf("silksdk: read input: %w", rerr)
		}
		bytesIn += 2

		nBytes := int16(uint16(sizeBuf[0]) | uint16(sizeBuf[1])<<8)
		if nBytes < 0 {
			break
		}
		nBytesUsize := int(nBytes)

		if nBytesUsize > len(payload) {
			payload = make([]byte, nBytesUsize)
		}
		if nBytesUsize > 0 {
			if _, rerr := io.ReadFull(src.Reader, payload[:nBytesUsize]); rerr != nil {
				return ErrTruncatedPacket
			}
			bytesIn += int64(nBytesUsize)
		}

		if nBytesUsize == 0 {
			frames := decControl.FramesPerPacket
			if frames <= 0 {
				frames = 1
			}
			for frameIdx := int32(0); frameIdx < frames; frameIdx++ {
				var outLen int32
				if rc := dec.Decode(&decState, &decControl, 1, payload, 0, out, &outLen); rc != 0 {
					cont, werr := sink.handleDecodeFailure(r.tolerant, frames-frameIdx, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return fmt.Errorf("silksdk: decode failed: %d", rc)
					}
					if r.tolerant == TolerantSilence {
						break
					}
					continue
				}
				if int(outLen) > len(out) {
					cont, werr := sink.handleDecodeFailure(r.tolerant, frames-frameIdx, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return errors.New("silksdk: decode frame too large")
					}
					if r.tolerant == TolerantSilence {
						break
					}
					continue
				}
				if werr := sink.writeSamples(out, int(outLen)); werr != nil {
					return werr
				}
				sink.frames++
			}
		} else {
			frames := 0
			for {
				var outLen int32
				if rc := dec.Decode(&decState, &decControl, 0, payload, int32(nBytesUsize), out, &outLen); rc != 0 {
					cont, werr := sink.handlePacketFailure(r.tolerant, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return fmt.Errorf("silksdk: decode failed: %d", rc)
					}
					break
				}
				if int(outLen) > len(out) {
					cont, werr := sink.handlePacketFailure(r.tolerant, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return errors.New("silksdk: decode frame too large")
					}
					break
				}
				if werr := sink.writeSamples(out, int(outLen)); werr != nil {
					return werr
				}
				sink.frames++
				frames++
				if frames > maxInputFrames {
					cont, werr := sink.handlePacketFailure(r.tolerant, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return errors.New("silksdk: too many frames in packet")
					}
					break
				}
				if decControl.MoreInternalDecoderFrames == 0 {
					break
				}
			}
		}
		packets++
	}

	if err := sink.finalize(); err != nil {
		return err
	}
	if opts.Metrics != nil {
		opts.Metrics.BytesIn = bytesIn
		opts.Metrics.SamplesOut = sink.samples
		opts.Metrics.PacketsIn = packets
		opts.Metrics.FramesOut = sink.frames
		if r.sampleRate > 0 {
			opts.Metrics.DurationMs = (sink.samples * 1000) / int64(r.sampleRate)
		}
		if refSamples != nil && sink.refEnergy > 0 {
			opts.Metrics.EnergyRatio = sink.energy / sink.refEnergy
			if sink.noiseEnergy > 0 {
				opts.Metrics.SNRDb = 10 * math.Log10(sink.refEnergy/sink.noiseEnergy)
			} else {
				opts.Metrics.SNRDb = math.Inf(1)
			}
		}
	}
	return nil
}

// decodeSink mirrors silkcli/output.go's outputSink, simplified for the
// SDK's Reader/Writer-only world (no file paths, no stderr printing).
type decodeSink struct {
	out         io.Writer
	wavBuffer   *bytes.Buffer // buffered PCM when WriteWAV is true
	writeWAV    bool
	sampleRate  int32
	wantStats   bool
	frames      int
	samples     int64
	refSamples  []int16
	refPos      int
	energy      float64
	refEnergy   float64
	noiseEnergy float64
}

func newSink(out io.Writer, writeWAV bool, sampleRate int32, wantStats bool, refSamples []int16) *decodeSink {
	s := &decodeSink{
		out:        out,
		writeWAV:   writeWAV,
		sampleRate: sampleRate,
		wantStats:  wantStats,
		refSamples: refSamples,
	}
	if writeWAV {
		// Write WAV header at finalize once we know the data length.
		// Buffer PCM in memory until then.
		s.wavBuffer = new(bytes.Buffer)
	}
	return s
}

func (s *decodeSink) writeSamples(buf []int16, length int) error {
	if length == 0 {
		return nil
	}
	bs := make([]byte, length*2)
	for i := 0; i < length; i++ {
		bs[2*i] = byte(uint16(buf[i]))
		bs[2*i+1] = byte(uint16(buf[i]) >> 8)
	}
	if s.wantStats {
		for i := 0; i < length; i++ {
			sample := float64(buf[i])
			s.energy += sample * sample
			s.samples++
			if s.refSamples != nil && s.refPos < len(s.refSamples) {
				rv := float64(s.refSamples[s.refPos])
				s.refEnergy += rv * rv
				diff := rv - sample
				s.noiseEnergy += diff * diff
				s.refPos++
			}
		}
	} else {
		s.samples += int64(length)
	}
	if s.writeWAV {
		if _, err := s.wavBuffer.Write(bs); err != nil {
			return fmt.Errorf("silksdk: buffer wav: %w", err)
		}
		return nil
	}
	if _, err := s.out.Write(bs); err != nil {
		return fmt.Errorf("silksdk: write output: %w", err)
	}
	return nil
}

func (s *decodeSink) writeSilence(nSamples int) error {
	if nSamples == 0 {
		return nil
	}
	bs := make([]byte, nSamples*2)
	if s.wantStats {
		s.samples += int64(nSamples)
		if s.refSamples != nil {
			for i := 0; i < nSamples; i++ {
				if s.refPos < len(s.refSamples) {
					rv := float64(s.refSamples[s.refPos])
					s.refEnergy += rv * rv
					s.noiseEnergy += rv * rv
					s.refPos++
				}
			}
		}
	} else {
		s.samples += int64(nSamples)
	}
	if s.writeWAV {
		if _, err := s.wavBuffer.Write(bs); err != nil {
			return fmt.Errorf("silksdk: buffer wav: %w", err)
		}
		return nil
	}
	if _, err := s.out.Write(bs); err != nil {
		return fmt.Errorf("silksdk: write output: %w", err)
	}
	return nil
}

func (s *decodeSink) handleDecodeFailure(mode TolerantMode, framesLeft int32, frameSamples int32) (bool, error) {
	switch mode {
	case TolerantSkip:
		return true, nil
	case TolerantSilence:
		if framesLeft > 0 {
			samples := int(framesLeft) * int(frameSamples)
			if err := s.writeSilence(samples); err != nil {
				return false, err
			}
			s.frames += int(framesLeft)
		}
		return true, nil
	default:
		return false, nil
	}
}

func (s *decodeSink) handlePacketFailure(mode TolerantMode, frameSamples int32) (bool, error) {
	switch mode {
	case TolerantSkip:
		return true, nil
	case TolerantSilence:
		if err := s.writeSilence(int(frameSamples)); err != nil {
			return false, err
		}
		s.frames++
		return true, nil
	default:
		return false, nil
	}
}

func (s *decodeSink) finalize() error {
	if !s.writeWAV {
		return nil
	}
	dataLen := uint32(s.wavBuffer.Len())
	if err := wav.WriteHeader(s.out, uint32(s.sampleRate), dataLen); err != nil {
		return fmt.Errorf("silksdk: write wav header: %w", err)
	}
	if _, err := s.out.Write(s.wavBuffer.Bytes()); err != nil {
		return fmt.Errorf("silksdk: write wav body: %w", err)
	}
	return nil
}

// readReferencePCM reads a full PCM stream (mono s16le or WAV) into an
// int16 slice for SNR measurement.
func readReferencePCM(r io.Reader, sampleRate int32) ([]int16, error) {
	src, err := wav.Open(r)
	if err != nil {
		return nil, err
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
	bs, err := io.ReadAll(src.Reader)
	if err != nil {
		return nil, err
	}
	if len(bs)%2 != 0 {
		bs = bs[:len(bs)-1]
	}
	samples := make([]int16, len(bs)/2)
	for i := range samples {
		samples[i] = int16(uint16(bs[2*i]) | uint16(bs[2*i+1])<<8)
	}
	return samples, nil
}
