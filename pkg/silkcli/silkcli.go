// Package silkcli houses the encode/decode logic and argument parsing for
// the silk-go CLI binary. The split lets cmd/silk-go/main.go stay small
// (just dispatch + usage) and keeps the per-subcommand implementations
// testable without spawning a process.
package silkcli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/rolandhe/silk-go/internal/silk"
	"github.com/rolandhe/silk-go/internal/silk/dec"
	"github.com/rolandhe/silk-go/internal/silk/enc"
	"github.com/rolandhe/silk-go/pkg/wav"
)

// SilkHeader is the magic prefix on a v3 SILK stream.
const SilkHeader = "#!SILK_V3"

// TencentPrefix is a one-byte 0x02 marker some downstream products
// prepend before the SILK header. Selectable on encode via --tencent;
// decode auto-detects.
const TencentPrefix byte = 0x02

const (
	maxBytesPerFrame = 250
	maxInputFrames   = 5
	frameLengthMs    = 20
	maxFrameLengthMs = 60
	maxAPIFsHz       = 48000
	maxInternalFsHz  = 24000
)

// EncodeArgs collects the parsed `encode` subcommand arguments.
type EncodeArgs struct {
	Input           string
	Output          string
	SampleRate      int32
	SampleRateSet   bool
	MaxInternalRate int32
	MaxInternalSet  bool
	PacketMs        int32
	Bitrate         int32
	Loss            int32
	FEC             int32
	DTX             int32
	Complexity      int32
	Tencent         bool
	Stats           bool
	Quiet           bool
}

// DecodeArgs collects the parsed `decode` subcommand arguments.
type DecodeArgs struct {
	Input      string
	Output     string
	SampleRate int32
	WAV        bool
	Tolerant   TolerantMode
	Stats      bool
	Reference  string
	Quiet      bool
}

// TolerantMode controls how the decoder reacts to broken packets.
type TolerantMode int

const (
	TolerantOff TolerantMode = iota
	TolerantSkip
	TolerantSilence
)

// PrintUsage writes the CLI help text to w. Mirrors rust-silk's print_usage.
func PrintUsage(w io.Writer) {
	fmt.Fprintln(w, `silk-go <encode|decode> [options]

encode options:
  -i, --input <path>           input PCM s16le or WAV (or - for stdin)
  -o, --output <path>          output .silk (or - for stdout)
  --sample-rate <Hz>           default 24000
  --max-internal <Hz>          default = min(sample-rate, 24000)
  --packet-ms <ms>             default 20
  --bitrate <bps>              default 25000
  --loss <percent>             default 0
  --fec <0|1>                   default 0
  --dtx <0|1>                   default 0
  --complexity <0|1|2>          default 2
  --tencent                     write 0x02 prefix + SILK header
  --stats                       print encoding stats
  --quiet                       suppress progress

decode options:
  -i, --input <path>           input .silk (or - for stdin)
  -o, --output <path>          output PCM s16le (or .wav, or - for stdout)
  --sample-rate <Hz>           default 24000
  --wav                         write WAV header
  --tolerant <skip|silence>     handle broken packets
  --metrics                     print decode metrics
  --reference <path>            compare against PCM/WAV reference
  --quiet                       suppress progress`)
}

// ParseEncodeArgs parses the encode subcommand from raw CLI tokens.
func ParseEncodeArgs(args []string) (*EncodeArgs, error) {
	a := &EncodeArgs{
		SampleRate:      24000,
		MaxInternalRate: 24000,
		PacketMs:        20,
		Bitrate:         25000,
		Complexity:      2,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-i", "--input":
			i++
			if i >= len(args) {
				return nil, errors.New("missing value for input")
			}
			a.Input = args[i]
		case "-o", "--output":
			i++
			if i >= len(args) {
				return nil, errors.New("missing value for output")
			}
			a.Output = args[i]
		case "--sample-rate", "--fs":
			v, err := nextInt32(args, &i, "sample-rate")
			if err != nil {
				return nil, err
			}
			a.SampleRate = v
			a.SampleRateSet = true
			if !a.MaxInternalSet {
				a.MaxInternalRate = defaultMaxInternalRate(a.SampleRate)
			}
		case "--max-internal":
			v, err := nextInt32(args, &i, "max-internal")
			if err != nil {
				return nil, err
			}
			a.MaxInternalRate = v
			a.MaxInternalSet = true
		case "--packet-ms":
			v, err := nextInt32(args, &i, "packet-ms")
			if err != nil {
				return nil, err
			}
			a.PacketMs = v
		case "--bitrate":
			v, err := nextInt32(args, &i, "bitrate")
			if err != nil {
				return nil, err
			}
			a.Bitrate = v
		case "--loss":
			v, err := nextInt32(args, &i, "loss")
			if err != nil {
				return nil, err
			}
			a.Loss = v
		case "--fec":
			v, err := nextInt32(args, &i, "fec")
			if err != nil {
				return nil, err
			}
			a.FEC = v
		case "--dtx":
			v, err := nextInt32(args, &i, "dtx")
			if err != nil {
				return nil, err
			}
			a.DTX = v
		case "--complexity":
			v, err := nextInt32(args, &i, "complexity")
			if err != nil {
				return nil, err
			}
			a.Complexity = v
		case "--tencent":
			a.Tencent = true
		case "--stats":
			a.Stats = true
		case "--quiet":
			a.Quiet = true
		case "-h", "--help":
			PrintUsage(os.Stderr)
			os.Exit(0)
		default:
			return nil, fmt.Errorf("unknown arg: %s", arg)
		}
	}
	if a.Input == "" {
		return nil, errors.New("missing --input")
	}
	if a.Output == "" {
		return nil, errors.New("missing --output")
	}
	return a, nil
}

// ParseDecodeArgs parses the decode subcommand from raw CLI tokens.
func ParseDecodeArgs(args []string) (*DecodeArgs, error) {
	a := &DecodeArgs{
		SampleRate: 24000,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-i", "--input":
			i++
			if i >= len(args) {
				return nil, errors.New("missing value for input")
			}
			a.Input = args[i]
		case "-o", "--output":
			i++
			if i >= len(args) {
				return nil, errors.New("missing value for output")
			}
			a.Output = args[i]
		case "--sample-rate", "--fs":
			v, err := nextInt32(args, &i, "sample-rate")
			if err != nil {
				return nil, err
			}
			a.SampleRate = v
		case "--wav":
			a.WAV = true
		case "--tolerant":
			i++
			if i >= len(args) {
				return nil, errors.New("missing value for tolerant")
			}
			switch args[i] {
			case "skip":
				a.Tolerant = TolerantSkip
			case "silence":
				a.Tolerant = TolerantSilence
			default:
				return nil, fmt.Errorf("invalid tolerant mode: %s", args[i])
			}
		case "--metrics":
			a.Stats = true
		case "--reference", "--ref":
			i++
			if i >= len(args) {
				return nil, errors.New("missing value for reference")
			}
			a.Reference = args[i]
			a.Stats = true
		case "--quiet":
			a.Quiet = true
		case "-h", "--help":
			PrintUsage(os.Stderr)
			os.Exit(0)
		default:
			return nil, fmt.Errorf("unknown arg: %s", arg)
		}
	}
	if a.Input == "" {
		return nil, errors.New("missing --input")
	}
	if a.Output == "" {
		return nil, errors.New("missing --output")
	}
	return a, nil
}

func nextInt32(args []string, i *int, name string) (int32, error) {
	*i++
	if *i >= len(args) {
		return 0, fmt.Errorf("missing value for %s", name)
	}
	v, err := strconv.ParseInt(args[*i], 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %s", name, args[*i])
	}
	return int32(v), nil
}

func defaultMaxInternalRate(sr int32) int32 {
	if sr < maxInternalFsHz {
		return sr
	}
	return maxInternalFsHz
}

func isSupportedSampleRate(rate int32) bool {
	switch rate {
	case 8000, 12000, 16000, 24000, 48000:
		return true
	}
	return false
}

func isSupportedInternalSampleRate(rate int32) bool {
	switch rate {
	case 8000, 12000, 16000, 24000:
		return true
	}
	return false
}

func hasWAVExtension(path string) bool {
	dot := strings.LastIndex(path, ".")
	if dot < 0 {
		return false
	}
	return strings.EqualFold(path[dot:], ".wav")
}

// openInput opens the input file (or stdin) and detects whether it's a
// raw PCM stream or a WAV. Returned reader is positioned right at the
// first sample.
func openInput(path string) (*wav.OpenResult, io.Closer, error) {
	if path == "-" {
		w, err := wav.Open(os.Stdin)
		return w, nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open input: %w", err)
	}
	w, err := wav.Open(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return w, f, nil
}

func openOutputWriter(path string) (io.WriteCloser, error) {
	if path == "-" {
		return nopCloser{os.Stdout}, nil
	}
	return os.Create(path)
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// Encode runs the encode subcommand: read PCM input → SILK packets.
func Encode(a *EncodeArgs) (err error) {
	src, closer, err := openInput(a.Input)
	if err != nil {
		return err
	}
	if closer != nil {
		defer func() {
			if cerr := closer.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()
	}
	if src.Info != nil {
		if a.SampleRateSet && a.SampleRate != src.Info.SampleRate {
			return fmt.Errorf("sample-rate (%d) does not match WAV (%d)", a.SampleRate, src.Info.SampleRate)
		}
		if !a.SampleRateSet {
			a.SampleRate = src.Info.SampleRate
		}
		if !a.MaxInternalSet {
			a.MaxInternalRate = defaultMaxInternalRate(a.SampleRate)
		}
	}

	if a.SampleRate <= 0 || a.SampleRate > maxAPIFsHz {
		return fmt.Errorf("sample-rate out of range (8000-48000): %d", a.SampleRate)
	}
	if !isSupportedSampleRate(a.SampleRate) {
		return fmt.Errorf("sample-rate must be one of 8000, 12000, 16000, 24000, 48000: %d", a.SampleRate)
	}
	if a.MaxInternalRate <= 0 || a.MaxInternalRate > maxInternalFsHz {
		return fmt.Errorf("max-internal out of range (8000-24000): %d", a.MaxInternalRate)
	}
	if !isSupportedInternalSampleRate(a.MaxInternalRate) {
		return fmt.Errorf("max-internal must be one of 8000, 12000, 16000, 24000: %d", a.MaxInternalRate)
	}
	if a.PacketMs%frameLengthMs != 0 {
		return fmt.Errorf("packet-ms must be multiple of %d: %d", frameLengthMs, a.PacketMs)
	}
	if a.PacketMs < frameLengthMs || a.PacketMs > frameLengthMs*maxInputFrames {
		return fmt.Errorf("packet-ms out of range (%d-%d): %d",
			frameLengthMs, frameLengthMs*maxInputFrames, a.PacketMs)
	}

	out, err := openOutputWriter(a.Output)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close output: %w", cerr)
		}
	}()

	var totalEncodedBytes uint64
	var totalInputSamples uint64

	if a.Tencent {
		if _, err := out.Write([]byte{TencentPrefix}); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		totalEncodedBytes++
	}
	if _, err := out.Write([]byte(SilkHeader)); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	totalEncodedBytes += uint64(len(SilkHeader))

	var encState enc.StateFIX
	if _, rc := enc.InitEncoder(&encState); rc != 0 {
		return fmt.Errorf("init encoder: %d", rc)
	}

	encControl := enc.EncControl{
		APISampleRate:         a.SampleRate,
		MaxInternalSampleRate: a.MaxInternalRate,
		PacketSize:            (a.PacketMs * a.SampleRate) / 1000,
		BitRate:               a.Bitrate,
		PacketLossPercentage:  a.Loss,
		Complexity:            a.Complexity,
		UseInBandFEC:          a.FEC,
		UseDTX:                a.DTX,
	}

	frameSamples := (frameLengthMs * a.SampleRate) / 1000
	smplsSinceLastPacket := int32(0)
	payload := make([]byte, maxBytesPerFrame*maxInputFrames)
	buffer := make([]byte, frameSamples*2)
	samples := make([]int16, frameSamples)
	packets := 0

	for {
		read := 0
		for read < len(buffer) {
			n, rerr := src.Reader.Read(buffer[read:])
			if n > 0 {
				read += n
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				return rerr
			}
		}
		if read == 0 {
			break
		}
		if read < len(buffer) {
			for i := read; i < len(buffer); i++ {
				buffer[i] = 0
			}
		}
		totalInputSamples += uint64(read / 2)
		for idx := 0; idx < int(frameSamples); idx++ {
			samples[idx] = int16(uint16(buffer[2*idx]) | uint16(buffer[2*idx+1])<<8)
		}

		nBytes := int32(maxBytesPerFrame * maxInputFrames)
		if rc := enc.Encode(&encState, &encControl, samples, frameSamples, payload, &nBytes); rc != 0 {
			return fmt.Errorf("encode failed: %d", rc)
		}

		smplsSinceLastPacket += frameSamples
		packetSizeMs := (1000 * encControl.PacketSize) / encControl.APISampleRate
		if (1000*smplsSinceLastPacket)/a.SampleRate == packetSizeMs {
			var sizeBuf [2]byte
			n16 := int16(nBytes)
			sizeBuf[0] = byte(n16)
			sizeBuf[1] = byte(n16 >> 8)
			if _, err := out.Write(sizeBuf[:]); err != nil {
				return fmt.Errorf("write output: %w", err)
			}
			totalEncodedBytes += 2
			if nBytes > 0 {
				if _, err := out.Write(payload[:nBytes]); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				totalEncodedBytes += uint64(nBytes)
			}
			smplsSinceLastPacket = 0
			packets++
			if !a.Quiet {
				fmt.Fprintf(os.Stderr, "\rPackets encoded: %d", packets)
			}
		}
	}

	if !a.Tencent {
		// EOF marker for non-tencent format: int16(-1).
		if _, err := out.Write([]byte{0xFF, 0xFF}); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		totalEncodedBytes += 2
	}

	if a.Stats {
		duration := float64(totalInputSamples) / float64(a.SampleRate)
		var bitrate float64
		if duration > 0 {
			bitrate = float64(totalEncodedBytes) * 8 / duration
		}
		fmt.Fprintf(os.Stderr,
			"stats: packets=%d samples=%d duration=%.3fs bytes=%d bitrate=%.1fbps\n",
			packets, totalInputSamples, duration, totalEncodedBytes, bitrate)
	}
	if !a.Quiet {
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

// Decode runs the decode subcommand: read SILK packets → PCM samples.
func Decode(a *DecodeArgs) (err error) {
	if a.SampleRate <= 0 || a.SampleRate > maxAPIFsHz {
		return fmt.Errorf("sample-rate out of range (8000-48000): %d", a.SampleRate)
	}
	if !isSupportedSampleRate(a.SampleRate) {
		return fmt.Errorf("sample-rate must be one of 8000, 12000, 16000, 24000, 48000: %d", a.SampleRate)
	}

	src, closer, err := openInput(a.Input)
	if err != nil {
		return err
	}
	if closer != nil {
		defer func() {
			if cerr := closer.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()
	}
	if src.Info != nil {
		return errors.New("input appears to be WAV; expected silk")
	}

	var refSamples []int16
	if a.Reference != "" {
		refSamples, err = loadReferenceSamples(a.Reference, a.SampleRate)
		if err != nil {
			return err
		}
	}

	withStats := a.Stats || a.Reference != ""
	sink, err := newOutputSink(a.Output, a.SampleRate, a.WAV, withStats, refSamples)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := sink.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close output: %w", cerr)
		}
	}()

	// SILK header detection: optional 0x02 prefix, then "#!SILK_V3".
	var first [1]byte
	if _, err := io.ReadFull(src.Reader, first[:]); err != nil {
		return errors.New("invalid silk header")
	}
	if first[0] == TencentPrefix {
		header := make([]byte, len(SilkHeader))
		if _, err := io.ReadFull(src.Reader, header); err != nil {
			return errors.New("invalid silk header")
		}
		if string(header) != SilkHeader {
			return errors.New("invalid silk header")
		}
	} else {
		header := make([]byte, len(SilkHeader)-1)
		if _, err := io.ReadFull(src.Reader, header); err != nil {
			return errors.New("invalid silk header")
		}
		if string(header) != SilkHeader[1:] {
			return errors.New("invalid silk header")
		}
	}

	var decState dec.State
	dec.InitDecoder(&decState)

	decControl := silk.DecControl{
		APISampleRate:   a.SampleRate,
		FramesPerPacket: 1,
	}

	const maxFrameSamples = silk.MaxAPIFsKHz * frameLengthMs
	out := make([]int16, maxFrameSamples)
	payload := make([]byte, maxBytesPerFrame*maxInputFrames)
	frameSamples := (frameLengthMs * a.SampleRate) / 1000
	packets := 0

	for {
		var sizeBuf [2]byte
		n, rerr := io.ReadFull(src.Reader, sizeBuf[:])
		if rerr == io.EOF || (rerr == io.ErrUnexpectedEOF && n == 0) {
			break
		}
		if rerr == io.ErrUnexpectedEOF {
			return errors.New("truncated packet size")
		}
		if rerr != nil {
			return rerr
		}
		nBytes := int16(uint16(sizeBuf[0]) | uint16(sizeBuf[1])<<8)
		if nBytes < 0 {
			break
		}
		nBytesUsize := int(nBytes)
		if sink.stats != nil {
			sink.stats.Packets++
			sink.stats.Bytes += uint64(nBytesUsize + 2)
		}
		if nBytesUsize > len(payload) {
			payload = make([]byte, nBytesUsize)
		}
		if nBytesUsize > 0 {
			if _, err := io.ReadFull(src.Reader, payload[:nBytesUsize]); err != nil {
				return errors.New("truncated payload")
			}
		}

		if nBytesUsize == 0 {
			frames := decControl.FramesPerPacket
			if frames <= 0 {
				frames = 1
			}
			for frameIdx := int32(0); frameIdx < frames; frameIdx++ {
				var outLen int32
				if rc := dec.Decode(&decState, &decControl, 1, payload, 0, out, &outLen); rc != 0 {
					cont, werr := sink.handleDecodeFailure(a.Tolerant, frames-frameIdx, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return fmt.Errorf("decode failed: %d", rc)
					}
					if a.Tolerant == TolerantSilence {
						break
					}
					continue
				}
				if int(outLen) > len(out) {
					cont, werr := sink.handleDecodeFailure(a.Tolerant, frames-frameIdx, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return errors.New("decode frame too large")
					}
					if a.Tolerant == TolerantSilence {
						break
					}
					continue
				}
				if err := sink.WriteSamples(out, int(outLen)); err != nil {
					return err
				}
				sink.RecordFrames(1)
			}
		} else {
			frames := 0
			for {
				var outLen int32
				if rc := dec.Decode(&decState, &decControl, 0, payload, int32(nBytesUsize), out, &outLen); rc != 0 {
					cont, werr := sink.handlePacketFailure(a.Tolerant, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return fmt.Errorf("decode failed: %d", rc)
					}
					break
				}
				if int(outLen) > len(out) {
					cont, werr := sink.handlePacketFailure(a.Tolerant, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return errors.New("decode frame too large")
					}
					break
				}
				if err := sink.WriteSamples(out, int(outLen)); err != nil {
					return err
				}
				sink.RecordFrames(1)
				frames++
				if frames > maxInputFrames {
					cont, werr := sink.handlePacketFailure(a.Tolerant, frameSamples)
					if werr != nil {
						return werr
					}
					if !cont {
						return errors.New("too many frames in packet")
					}
					break
				}
				if decControl.MoreInternalDecoderFrames == 0 {
					break
				}
			}
		}
		packets++
		if !a.Quiet {
			fmt.Fprintf(os.Stderr, "\rPackets decoded: %d", packets)
		}
	}

	if err := sink.Finalize(); err != nil {
		return err
	}
	if sink.stats != nil {
		sink.PrintStats(a.SampleRate)
	}
	if !a.Quiet {
		fmt.Fprintln(os.Stderr)
	}
	return nil
}
