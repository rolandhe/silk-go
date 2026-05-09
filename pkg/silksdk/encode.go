package silksdk

import (
	"fmt"
	"io"
	"time"

	"github.com/rolandhe/silk-go/internal/silk/enc"
	"github.com/rolandhe/silk-go/pkg/wav"
)

// Encode reads PCM samples from opts.Input and writes a SILK byte stream
// to opts.Output. The output bytes are byte-identical to running
// `silk-go encode` (and rust-silk's CLI) with equivalent flags.
//
// Encode does not close opts.Output. The caller owns flush/close.
//
// If opts.Input is a WAV stream (RIFF/WAVE header), the SDK auto-strips
// the header and (when opts.SampleRate is unset) uses the embedded rate.
// Mismatched explicit SampleRate vs WAV rate returns an error.
func Encode(opts EncodeOptions) error {
	startedAt := time.Now()

	if opts.Input == nil {
		return fmt.Errorf("%w: Input is required", ErrInvalidOption)
	}
	if opts.Output == nil {
		return fmt.Errorf("%w: Output is required", ErrInvalidOption)
	}

	src, err := wav.Open(opts.Input)
	if err != nil {
		return fmt.Errorf("silksdk: read input: %w", err)
	}
	hasWAV := src.Info != nil
	var wavRate int32
	if hasWAV {
		wavRate = src.Info.SampleRate
	}

	r, err := opts.resolveEncode(wavRate, hasWAV)
	if err != nil {
		return err
	}

	out := opts.Output
	stats := opts.Stats

	var bytesOut int64

	if r.tencent {
		if _, werr := out.Write([]byte{tencentPrefix}); werr != nil {
			return fmt.Errorf("silksdk: write output: %w", werr)
		}
		bytesOut++
	}
	if _, werr := out.Write([]byte(silkHeader)); werr != nil {
		return fmt.Errorf("silksdk: write output: %w", werr)
	}
	bytesOut += int64(len(silkHeader))

	var encState enc.StateFIX
	if _, rc := enc.InitEncoder(&encState); rc != 0 {
		return fmt.Errorf("silksdk: init encoder: %d", rc)
	}

	encControl := enc.EncControl{
		APISampleRate:         r.sampleRate,
		MaxInternalSampleRate: r.maxInternal,
		PacketSize:            (r.packetMs * r.sampleRate) / 1000,
		BitRate:               r.bitRate,
		PacketLossPercentage:  r.loss,
		Complexity:            r.complexity,
		UseInBandFEC:          r.fec,
		UseDTX:                r.dtx,
	}

	frameSamples := (frameLengthMs * r.sampleRate) / 1000
	smplsSinceLastPacket := int32(0)
	payload := make([]byte, maxBytesPerFrame*maxInputFrames)
	buffer := make([]byte, frameSamples*2)
	samples := make([]int16, frameSamples)
	packets := 0
	var samplesIn int64

	for {
		// Fill one 20 ms frame.
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
				return fmt.Errorf("silksdk: read input: %w", rerr)
			}
		}
		if read == 0 {
			break
		}
		// Zero-pad the partial trailing frame to match CLI behaviour.
		if read < len(buffer) {
			for i := read; i < len(buffer); i++ {
				buffer[i] = 0
			}
		}
		samplesIn += int64(read / 2)
		for idx := 0; idx < int(frameSamples); idx++ {
			samples[idx] = int16(uint16(buffer[2*idx]) | uint16(buffer[2*idx+1])<<8)
		}

		nBytes := int32(maxBytesPerFrame * maxInputFrames)
		if rc := enc.Encode(&encState, &encControl, samples, frameSamples, payload, &nBytes); rc != 0 {
			return fmt.Errorf("silksdk: encode failed: %d", rc)
		}

		smplsSinceLastPacket += frameSamples
		packetSizeMs := (1000 * encControl.PacketSize) / encControl.APISampleRate
		if (1000*smplsSinceLastPacket)/r.sampleRate == packetSizeMs {
			var sizeBuf [2]byte
			n16 := int16(nBytes)
			sizeBuf[0] = byte(n16)
			sizeBuf[1] = byte(n16 >> 8)
			if _, werr := out.Write(sizeBuf[:]); werr != nil {
				return fmt.Errorf("silksdk: write output: %w", werr)
			}
			bytesOut += 2
			if nBytes > 0 {
				if _, werr := out.Write(payload[:nBytes]); werr != nil {
					return fmt.Errorf("silksdk: write output: %w", werr)
				}
				bytesOut += int64(nBytes)
			}
			smplsSinceLastPacket = 0
			packets++
		}
	}

	if !r.tencent {
		// EOF marker int16(-1) for non-tencent format.
		if _, werr := out.Write([]byte{0xFF, 0xFF}); werr != nil {
			return fmt.Errorf("silksdk: write output: %w", werr)
		}
		bytesOut += 2
	}

	if stats != nil {
		stats.BytesIn = samplesIn * 2
		stats.SamplesIn = samplesIn
		stats.BytesOut = bytesOut
		stats.PacketsOut = packets
		if r.sampleRate > 0 {
			stats.DurationMs = (samplesIn * 1000) / int64(r.sampleRate)
		}
		stats.EncodeWallNs = time.Since(startedAt).Nanoseconds()
	}
	return nil
}
