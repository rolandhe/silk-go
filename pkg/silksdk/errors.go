package silksdk

import "errors"

// ErrInvalidOption is returned when the EncodeOptions or DecodeOptions
// fail validation. Wrap with %w-friendly fmt.Errorf to add context.
var ErrInvalidOption = errors.New("silksdk: invalid option")

// ErrInvalidSilkHeader is returned by Decode when the input stream does
// not start with the expected SILK_V3 magic (with optional 0x02 Tencent
// prefix).
var ErrInvalidSilkHeader = errors.New("silksdk: invalid silk header")

// ErrTruncatedPacket is returned by Decode when the byte stream ends in
// the middle of a packet — i.e. the size prefix arrived but the payload
// did not.
var ErrTruncatedPacket = errors.New("silksdk: truncated packet")

// ErrUnexpectedWAVInput is returned by Decode when the silk Input stream
// looks like a WAV file (it should be a .silk byte stream).
var ErrUnexpectedWAVInput = errors.New("silksdk: input appears to be WAV; expected silk")
