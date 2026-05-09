package silksdk

import (
	"fmt"
	"io"
)

// Internal constants mirroring pkg/silkcli. Kept private so the SDK
// surface stays minimal.
const (
	silkHeader       = "#!SILK_V3"
	tencentPrefix    = byte(0x02)
	frameLengthMs    = 20
	maxBytesPerFrame = 250
	maxInputFrames   = 5
	maxAPIFsHz       = 48000
	maxInternalFsHz  = 24000
)

// EncodeOptions configures one Encode call. The zero value is invalid —
// at minimum SampleRate must be set. Other zero-valued fields are filled
// with defaults documented per-field.
//
// Output bytes are byte-identical to running `silk-go encode` with the
// equivalent flags.
type EncodeOptions struct {
	// Input is the PCM source. Mono signed 16-bit little-endian; if the
	// stream begins with a RIFF/WAVE header (mono PCM 16-bit only), the
	// SDK strips it and uses the embedded SampleRate when EncodeOptions
	// did not set it explicitly. Required.
	Input io.Reader

	// Output receives the .silk byte stream (header + length-prefixed
	// payloads + optional EOF marker). May be a *bytes.Buffer for pure
	// in-memory operation; the SDK never closes Output. Required.
	Output io.Writer

	// SampleRate is the API sample rate in Hz: one of 8000, 12000, 16000,
	// 24000, 48000. When the Input is a WAV with a different rate the SDK
	// returns an error; when SampleRate is 0 and the Input is a WAV the
	// embedded rate is used. Otherwise required.
	SampleRate int

	// MaxInternalRate is the max internal codec rate in Hz: 8000, 12000,
	// 16000, or 24000. 0 selects min(SampleRate, 24000).
	MaxInternalRate int

	// PacketMs is the packet duration in milliseconds: 20, 40, 60, 80,
	// or 100. 0 selects 20.
	PacketMs int

	// BitRate is the target bitrate in bits/sec; clamped to [5000, 100000]
	// internally. 0 selects 25000.
	BitRate int

	// LossPercent hints expected packet-loss percentage to the encoder
	// for FEC/redundancy decisions; 0..100. Default 0.
	LossPercent int

	// UseFEC enables in-band Forward Error Correction.
	UseFEC bool

	// UseDTX enables Discontinuous Transmission (suppresses output for
	// silent frames after a warm-up).
	UseDTX bool

	// Complexity selects encoder complexity: 0/1/2 (higher = better
	// quality, more CPU). 0..2; default 2 when this field is 0.
	Complexity int

	// ComplexitySet, when true, disambiguates Complexity == 0 from the
	// "use default" zero value. Set this if you want complexity 0
	// explicitly. (Without it, Complexity == 0 is treated as default.)
	ComplexitySet bool

	// Tencent emits a leading 0x02 byte and omits the 0xFF 0xFF EOF
	// marker. Used by some Tencent products (QQ/WeChat voice).
	Tencent bool

	// Stats, when non-nil, is filled with byte/sample/packet/duration
	// counters by Encode. The SDK never prints — callers display.
	Stats *EncodeStats
}

// EncodeStats holds optional encode-side counters populated when an
// EncodeOptions.Stats pointer is supplied.
type EncodeStats struct {
	BytesIn      int64 // raw PCM bytes consumed (post-WAV-strip)
	BytesOut     int64 // .silk bytes written (incl. header + framing + EOF)
	PacketsOut   int   // number of payload packets emitted
	SamplesIn    int64 // PCM samples (one channel) consumed
	DurationMs   int64 // audio duration encoded (= SamplesIn*1000/SampleRate)
	EncodeWallNs int64 // wall-clock nanoseconds spent inside Encode
}

// DecodeOptions configures one Decode call. The zero value is invalid —
// at minimum SampleRate, Input, and Output must be set.
type DecodeOptions struct {
	// Input is the .silk byte stream (with or without the optional 0x02
	// Tencent prefix; auto-detected). Required.
	Input io.Reader

	// Output receives the decoded PCM (signed 16-bit little-endian, mono).
	// If WriteWAV is true, a 44-byte WAV header is emitted first. May be a
	// *bytes.Buffer; the SDK never closes Output. Required.
	Output io.Writer

	// SampleRate is the API sample rate of the produced PCM in Hz: 8000,
	// 12000, 16000, 24000, or 48000. Required.
	SampleRate int

	// WriteWAV prepends a WAV header to Output so the result is a valid
	// .wav file. Because the final sample count is unknown until decode
	// completes, the SDK buffers all PCM in memory and writes the header
	// last. (See the docs on Output for memory implications.)
	WriteWAV bool

	// Tolerant controls how Decode reacts to a corrupt packet. Default
	// (TolerantOff) returns an error on the first failure.
	Tolerant TolerantMode

	// Reference is an optional reference PCM stream (mono s16le at the
	// same SampleRate). When non-nil, Decode populates Metrics.SNRDb after
	// finishing. Reference content past SamplesOut is ignored; Reference
	// shorter than the decoded output rolls SNR over the available
	// reference samples only.
	Reference io.Reader

	// Metrics, when non-nil, is filled with byte/sample/packet counters
	// (and SNRDb if Reference != nil).
	Metrics *DecodeMetrics
}

// TolerantMode controls how Decode reacts to corrupt packets.
type TolerantMode int

const (
	// TolerantOff: any decode error returns immediately. (Default.)
	TolerantOff TolerantMode = iota

	// TolerantSkip: skip a corrupt frame; produce no PCM for that frame.
	TolerantSkip

	// TolerantSilence: insert a frame's worth of silence (zeros) for any
	// corrupt or unrecoverable frame.
	TolerantSilence
)

// DecodeMetrics holds optional decode-side counters populated when a
// DecodeOptions.Metrics pointer is supplied.
type DecodeMetrics struct {
	BytesIn     int64   // bytes consumed from Input (incl. header + framing)
	SamplesOut  int64   // PCM samples written to Output (one channel)
	PacketsIn   int     // number of packets parsed from Input
	FramesOut   int     // number of decoded frames (a packet may produce many)
	DurationMs  int64   // audio duration emitted (SamplesOut*1000/SampleRate)
	SNRDb       float64 // SNR in dB vs Reference; only set when Reference != nil
	EnergyRatio float64 // decoded-energy / reference-energy; only when Reference != nil
}

// resolveEncode applies defaults and validates an EncodeOptions, returning a
// snapshot whose required fields are filled in.
func (o EncodeOptions) resolveEncode(wavRate int32, hasWAV bool) (resolvedEncode, error) {
	r := resolvedEncode{}
	if o.Input == nil {
		return r, fmt.Errorf("%w: Input is required", ErrInvalidOption)
	}
	if o.Output == nil {
		return r, fmt.Errorf("%w: Output is required", ErrInvalidOption)
	}

	rate := int32(o.SampleRate)
	if hasWAV {
		if rate != 0 && rate != wavRate {
			return r, fmt.Errorf("%w: SampleRate (%d) does not match WAV (%d)", ErrInvalidOption, rate, wavRate)
		}
		if rate == 0 {
			rate = wavRate
		}
	}
	if rate == 0 {
		return r, fmt.Errorf("%w: SampleRate is required", ErrInvalidOption)
	}
	if !isSupportedAPISampleRate(rate) {
		return r, fmt.Errorf("%w: SampleRate must be one of 8000/12000/16000/24000/48000: %d", ErrInvalidOption, rate)
	}

	maxInternal := int32(o.MaxInternalRate)
	if maxInternal == 0 {
		if rate < maxInternalFsHz {
			maxInternal = rate
		} else {
			maxInternal = maxInternalFsHz
		}
	}
	if !isSupportedInternalSampleRate(maxInternal) {
		return r, fmt.Errorf("%w: MaxInternalRate must be one of 8000/12000/16000/24000: %d", ErrInvalidOption, maxInternal)
	}

	packetMs := int32(o.PacketMs)
	if packetMs == 0 {
		packetMs = frameLengthMs
	}
	if packetMs%frameLengthMs != 0 {
		return r, fmt.Errorf("%w: PacketMs must be a multiple of %d: %d", ErrInvalidOption, frameLengthMs, packetMs)
	}
	if packetMs < frameLengthMs || packetMs > frameLengthMs*maxInputFrames {
		return r, fmt.Errorf("%w: PacketMs out of range (%d..%d): %d", ErrInvalidOption, frameLengthMs, frameLengthMs*maxInputFrames, packetMs)
	}

	bitRate := int32(o.BitRate)
	if bitRate == 0 {
		bitRate = 25000
	}

	loss := int32(o.LossPercent)
	if loss < 0 || loss > 100 {
		return r, fmt.Errorf("%w: LossPercent out of range (0..100): %d", ErrInvalidOption, loss)
	}

	complexity := int32(o.Complexity)
	if !o.ComplexitySet && complexity == 0 {
		complexity = 2
	}
	if complexity < 0 || complexity > 2 {
		return r, fmt.Errorf("%w: Complexity must be 0/1/2: %d", ErrInvalidOption, complexity)
	}

	r.sampleRate = rate
	r.maxInternal = maxInternal
	r.packetMs = packetMs
	r.bitRate = bitRate
	r.loss = loss
	r.complexity = complexity
	if o.UseFEC {
		r.fec = 1
	}
	if o.UseDTX {
		r.dtx = 1
	}
	r.tencent = o.Tencent
	return r, nil
}

type resolvedEncode struct {
	sampleRate  int32
	maxInternal int32
	packetMs    int32
	bitRate     int32
	loss        int32
	complexity  int32
	fec         int32
	dtx         int32
	tencent     bool
}

func (o DecodeOptions) resolveDecode() (resolvedDecode, error) {
	r := resolvedDecode{}
	if o.Input == nil {
		return r, fmt.Errorf("%w: Input is required", ErrInvalidOption)
	}
	if o.Output == nil {
		return r, fmt.Errorf("%w: Output is required", ErrInvalidOption)
	}
	rate := int32(o.SampleRate)
	if rate == 0 {
		return r, fmt.Errorf("%w: SampleRate is required", ErrInvalidOption)
	}
	if !isSupportedAPISampleRate(rate) {
		return r, fmt.Errorf("%w: SampleRate must be one of 8000/12000/16000/24000/48000: %d", ErrInvalidOption, rate)
	}
	switch o.Tolerant {
	case TolerantOff, TolerantSkip, TolerantSilence:
	default:
		return r, fmt.Errorf("%w: invalid Tolerant mode: %d", ErrInvalidOption, o.Tolerant)
	}
	r.sampleRate = rate
	r.writeWAV = o.WriteWAV
	r.tolerant = o.Tolerant
	return r, nil
}

type resolvedDecode struct {
	sampleRate int32
	writeWAV   bool
	tolerant   TolerantMode
}

func isSupportedAPISampleRate(hz int32) bool {
	switch hz {
	case 8000, 12000, 16000, 24000, 48000:
		return true
	}
	return false
}

func isSupportedInternalSampleRate(hz int32) bool {
	switch hz {
	case 8000, 12000, 16000, 24000:
		return true
	}
	return false
}
