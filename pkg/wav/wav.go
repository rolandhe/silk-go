// Package wav implements minimal WAV header reading/writing for the
// silk-go CLI. Only mono 16-bit PCM is supported (matching the rust-silk
// CLI it replaces). Larger feature surface — multi-channel, float
// samples, BWF metadata — is intentionally out of scope.
package wav

import (
	"encoding/binary"
	"errors"
	"io"
)

// Info describes the format chunk of a parsed WAV file.
type Info struct {
	SampleRate    int32
	Channels      uint16
	BitsPerSample uint16
}

// PeekedReader is an io.Reader that has had a small prefix already read
// off it. Returned by Open when the input is a raw PCM stream so the
// caller can replay the prefix before continuing.
type PeekedReader struct {
	prefix []byte
	rest   io.Reader
}

func (p *PeekedReader) Read(buf []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(buf, p.prefix)
		p.prefix = p.prefix[n:]
		return n, nil
	}
	return p.rest.Read(buf)
}

// OpenResult is what Open returns: the (wrapped) reader and — when the
// input was a WAV file — its format info plus the data-chunk byte budget
// (a fresh reader limited to that budget so the caller can read until EOF
// without spilling into post-data chunks).
type OpenResult struct {
	Reader io.Reader
	Info   *Info // nil when input was raw PCM
}

// Open inspects the first 12 bytes of `r`. If they look like a RIFF/WAVE
// header it parses the format chunk, returns the data chunk wrapped in
// io.LimitReader. Otherwise it returns the input unchanged (with the
// prefix replayed in front).
func Open(r io.Reader) (*OpenResult, error) {
	prefix := make([]byte, 12)
	n, err := io.ReadFull(r, prefix)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		// Less than 12 bytes available — definitely not a WAV.
		if n == 0 {
			return nil, errors.New("empty input")
		}
		return &OpenResult{
			Reader: &PeekedReader{prefix: prefix[:n], rest: r},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if string(prefix[0:4]) == "RIFF" && string(prefix[8:12]) == "WAVE" {
		info, dataLen, err := parseHeader(r)
		if err != nil {
			return nil, err
		}
		return &OpenResult{
			Reader: io.LimitReader(r, int64(dataLen)),
			Info:   info,
		}, nil
	}
	return &OpenResult{
		Reader: &PeekedReader{prefix: prefix, rest: r},
	}, nil
}

// parseHeader walks the RIFF chunks (after the 12-byte RIFF/WAVE preamble
// has been consumed) until it finds `data`, returning the fmt info and
// the data chunk's byte length.
func parseHeader(r io.Reader) (*Info, uint32, error) {
	var fmtInfo *Info

	for {
		var chunkID [4]byte
		if _, err := io.ReadFull(r, chunkID[:]); err != nil {
			return nil, 0, errors.New("wav: missing chunk header")
		}
		var size uint32
		if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
			return nil, 0, errors.New("wav: truncated chunk size")
		}

		switch string(chunkID[:]) {
		case "fmt ":
			if size < 16 {
				return nil, 0, errors.New("wav: fmt chunk too small")
			}
			// Cap fmt chunks at 64 bytes — real WAVs use 16 (PCM) or 18
			// (extended) or 40 (extensible). A larger declared size is
			// either malformed or a DoS attempt.
			if size > 64 {
				return nil, 0, errors.New("wav: fmt chunk too large")
			}
			buf := make([]byte, size)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, 0, errors.New("wav: truncated fmt chunk")
			}
			audioFormat := binary.LittleEndian.Uint16(buf[0:2])
			channels := binary.LittleEndian.Uint16(buf[2:4])
			sampleRate := binary.LittleEndian.Uint32(buf[4:8])
			bitsPerSample := binary.LittleEndian.Uint16(buf[14:16])
			if audioFormat != 1 {
				return nil, 0, errors.New("wav: only PCM format is supported")
			}
			if channels != 1 {
				return nil, 0, errors.New("wav: only mono is supported")
			}
			if bitsPerSample != 16 {
				return nil, 0, errors.New("wav: only 16-bit PCM supported")
			}
			fmtInfo = &Info{
				SampleRate:    int32(sampleRate),
				Channels:      channels,
				BitsPerSample: bitsPerSample,
			}
		case "data":
			if fmtInfo == nil {
				return nil, 0, errors.New("wav: missing fmt chunk")
			}
			return fmtInfo, size, nil
		default:
			if err := skipBytes(r, size); err != nil {
				return nil, 0, err
			}
		}
		if size%2 == 1 {
			if err := skipBytes(r, 1); err != nil {
				return nil, 0, err
			}
		}
	}
}

func skipBytes(r io.Reader, size uint32) error {
	if size == 0 {
		return nil
	}
	if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
		return errors.New("wav: truncated chunk")
	}
	return nil
}

// WriteHeader writes a 44-byte WAV header for `dataLen` bytes of mono
// 16-bit PCM data. Used twice in the CLI's decode path: once with
// dataLen=0 as a placeholder, then once at the end with the real count
// (after seeking back to the start of the file).
func WriteHeader(w io.Writer, sampleRate uint32, dataLen uint32) error {
	byteRate := sampleRate * 2
	const blockAlign uint16 = 2
	const bitsPerSample uint16 = 16
	riffSize := uint32(36) + dataLen
	if riffSize < dataLen {
		riffSize = 0xFFFFFFFF // overflow guard, rust-silk uses saturating_add
	}
	var buf [44]byte
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], riffSize)
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(buf[22:24], 1) // mono
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], byteRate)
	binary.LittleEndian.PutUint16(buf[32:34], blockAlign)
	binary.LittleEndian.PutUint16(buf[34:36], bitsPerSample)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataLen)
	_, err := w.Write(buf[:])
	return err
}

// HeaderSize is the size of the fixed PCM/mono/16-bit WAV header.
const HeaderSize = 44
