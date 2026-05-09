// Package rangecoder implements the SILK range encoder/decoder ported from
// vendor/silk/src/SKP_Silk_range_coder.c.
//
// All buffer reads and writes are bounds-checked through the State.bufferLength
// field; on out-of-range conditions, the State.Error field is set and further
// operations become no-ops (matching the C reference).
package rangecoder

import "github.com/rolandhe/silk-go/internal/silk"

// State mirrors SKP_Silk_range_coder_state from vendor/silk/src/SKP_Silk_structs.h.
type State struct {
	BufferLength int32
	BufferIx     int32
	BaseQ32      uint32
	RangeQ16     uint32
	Error        int32
	Buffer       [silk.MaxArithmBytes]uint8
}

// EncInit — SKP_Silk_range_enc_init.
func (s *State) EncInit() {
	s.BufferLength = silk.MaxArithmBytes
	s.RangeQ16 = 0x0000FFFF
	s.BufferIx = 0
	s.BaseQ32 = 0
	s.Error = 0
}

// DecInit — SKP_Silk_range_dec_init. Copies the input bytes into State.Buffer.
func (s *State) DecInit(buf []byte) {
	if len(buf) > silk.MaxArithmBytes {
		s.Error = silk.RangeCoderDecPayloadTooLong
		return
	}
	copy(s.Buffer[:], buf)
	s.BufferLength = int32(len(buf))
	s.BufferIx = 0
	if len(buf) >= 4 {
		s.BaseQ32 = uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	} else {
		var b [4]byte
		copy(b[:], buf)
		s.BaseQ32 = uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	}
	s.RangeQ16 = 0x0000FFFF
	s.Error = 0
}

// Encode — SKP_Silk_range_encoder. Encodes a single symbol given its CDF.
func (s *State) Encode(data int32, prob []uint16) {
	if s.Error != 0 {
		return
	}
	baseQ32 := s.BaseQ32
	rangeQ16 := s.RangeQ16
	bufferIx := s.BufferIx

	lowQ16 := uint32(prob[data])
	highQ16 := uint32(prob[data+1])
	baseTmp := baseQ32
	baseQ32 += rangeQ16 * lowQ16
	rangeQ32 := rangeQ16 * (highQ16 - lowQ16)

	if baseQ32 < baseTmp {
		// Carry propagation. The C source assumes the carry chain
		// never reaches the start of the buffer; a malformed encoder
		// state could drive ix below 0 and panic. Guard explicitly.
		ix := bufferIx
		for ix > 0 {
			ix--
			s.Buffer[ix]++
			if s.Buffer[ix] != 0 {
				break
			}
		}
	}

	if rangeQ32&0xFF000000 != 0 {
		rangeQ16 = rangeQ32 >> 16
	} else {
		if rangeQ32&0xFFFF0000 != 0 {
			rangeQ16 = rangeQ32 >> 8
		} else {
			rangeQ16 = rangeQ32
			if bufferIx >= s.BufferLength {
				s.Error = silk.RangeCoderWriteBeyondBuffer
				return
			}
			s.Buffer[bufferIx] = uint8(baseQ32 >> 24)
			bufferIx++
			baseQ32 <<= 8
		}
		if bufferIx >= s.BufferLength {
			s.Error = silk.RangeCoderWriteBeyondBuffer
			return
		}
		s.Buffer[bufferIx] = uint8(baseQ32 >> 24)
		bufferIx++
		baseQ32 <<= 8
	}

	s.BaseQ32 = baseQ32
	s.RangeQ16 = rangeQ16
	s.BufferIx = bufferIx
}

// EncodeMulti — SKP_Silk_range_encoder_multi.
func (s *State) EncodeMulti(data []int32, probs [][]uint16) {
	for k := 0; k < len(data); k++ {
		s.Encode(data[k], probs[k])
	}
}

// Decode — SKP_Silk_range_decoder. Decodes a single symbol; probIx is the
// initial (middle) cdf index.
func (s *State) Decode(prob []uint16, probIx int32) (data int32) {
	if s.Error != 0 {
		return 0
	}
	baseQ32 := s.BaseQ32
	rangeQ16 := s.RangeQ16
	bufferIx := s.BufferIx

	highQ16 := uint32(prob[probIx])
	baseTmp := rangeQ16 * highQ16
	var lowQ16 uint32
	if baseTmp > baseQ32 {
		for {
			probIx--
			lowQ16 = uint32(prob[probIx])
			baseTmp = rangeQ16 * lowQ16
			if baseTmp <= baseQ32 {
				break
			}
			highQ16 = lowQ16
			if highQ16 == 0 {
				s.Error = silk.RangeCoderCDFOutOfRange
				return 0
			}
		}
	} else {
		for {
			lowQ16 = highQ16
			probIx++
			highQ16 = uint32(prob[probIx])
			baseTmp = rangeQ16 * highQ16
			if baseTmp > baseQ32 {
				probIx--
				break
			}
			if highQ16 == 0xFFFF {
				s.Error = silk.RangeCoderCDFOutOfRange
				return 0
			}
		}
	}
	data = probIx
	baseQ32 -= rangeQ16 * lowQ16
	rangeQ32 := rangeQ16 * (highQ16 - lowQ16)

	if rangeQ32&0xFF000000 != 0 {
		rangeQ16 = rangeQ32 >> 16
	} else {
		if rangeQ32&0xFFFF0000 != 0 {
			rangeQ16 = rangeQ32 >> 8
			if (baseQ32 >> 24) != 0 {
				s.Error = silk.RangeCoderNormalizationFail
				return 0
			}
		} else {
			rangeQ16 = rangeQ32
			if (baseQ32 >> 16) != 0 {
				s.Error = silk.RangeCoderNormalizationFail
				return 0
			}
			baseQ32 <<= 8
			// The C version reads from &buffer[4]: i.e. byte at index bufferIx+4.
			if bufferIx < s.BufferLength {
				baseQ32 |= uint32(s.bufferAt(bufferIx))
				bufferIx++
			}
		}
		baseQ32 <<= 8
		if bufferIx < s.BufferLength {
			baseQ32 |= uint32(s.bufferAt(bufferIx))
			bufferIx++
		}
	}

	if rangeQ16 == 0 {
		s.Error = silk.RangeCoderZeroIntervalWidth
		return 0
	}

	s.BaseQ32 = baseQ32
	s.RangeQ16 = rangeQ16
	s.BufferIx = bufferIx
	return data
}

// bufferAt returns Buffer[ix+4] without panicking when out-of-range
// (mirrors the C `buffer = &psRC->buffer[4]` pointer arithmetic combined
// with the bufferIx < bufferLength guard, which protects against ix+4
// landing past the array but also requires us to clamp).
func (s *State) bufferAt(ix int32) byte {
	at := ix + 4
	if at < 0 || int(at) >= len(s.Buffer) {
		return 0
	}
	return s.Buffer[at]
}

// DecodeMulti — SKP_Silk_range_decoder_multi.
func (s *State) DecodeMulti(probs [][]uint16, probStartIx []int32) []int32 {
	out := make([]int32, len(probs))
	for k := range probs {
		out[k] = s.Decode(probs[k], probStartIx[k])
	}
	return out
}

// GetLength — SKP_Silk_range_coder_get_length. Returns total bits and (out)
// total bytes in the bit-stream so far.
func (s *State) GetLength() (nBits int32, nBytes int32) {
	nBits = (s.BufferIx << 3) + clz32(s.RangeQ16-1) - 14
	nBytes = (nBits + 7) >> 3
	return
}

// EncWrapUp — SKP_Silk_range_enc_wrap_up.
func (s *State) EncWrapUp() {
	baseQ24 := s.BaseQ32 >> 8
	bitsInStream, nBytes := s.GetLength()
	bitsToStore := bitsInStream - (s.BufferIx << 3)
	baseQ24 += 0x00800000 >> uint(bitsToStore-1)
	baseQ24 &= uint32(0xFFFFFFFF) << uint(24-bitsToStore)

	if baseQ24&0x01000000 != 0 {
		// Carry propagation.
		ix := s.BufferIx
		for {
			ix--
			s.Buffer[ix]++
			if s.Buffer[ix] != 0 {
				break
			}
		}
	}

	if s.BufferIx < s.BufferLength {
		s.Buffer[s.BufferIx] = uint8(baseQ24 >> 16)
		s.BufferIx++
		if bitsToStore > 8 {
			if s.BufferIx < s.BufferLength {
				s.Buffer[s.BufferIx] = uint8(baseQ24 >> 8)
				s.BufferIx++
			}
		}
	}

	if bitsInStream&7 != 0 {
		mask := byte(0xFF >> uint(bitsInStream&7))
		if nBytes-1 < s.BufferLength {
			s.Buffer[nBytes-1] |= mask
		}
	}
}

// CheckAfterDecoding — SKP_Silk_range_coder_check_after_decoding.
func (s *State) CheckAfterDecoding() {
	bitsInStream, nBytes := s.GetLength()
	if nBytes-1 >= s.BufferLength {
		s.Error = silk.RangeCoderDecoderCheckFailed
		return
	}
	if bitsInStream&7 != 0 {
		mask := byte(0xFF >> uint(bitsInStream&7))
		if (s.Buffer[nBytes-1] & mask) != mask {
			s.Error = silk.RangeCoderDecoderCheckFailed
			return
		}
	}
}

func clz32(x uint32) int32 {
	if x == 0 {
		return 32
	}
	n := int32(0)
	if x&0xFFFF0000 == 0 {
		n += 16
		x <<= 16
	}
	if x&0xFF000000 == 0 {
		n += 8
		x <<= 8
	}
	if x&0xF0000000 == 0 {
		n += 4
		x <<= 4
	}
	if x&0xC0000000 == 0 {
		n += 2
		x <<= 2
	}
	if x&0x80000000 == 0 {
		n++
	}
	return n
}
