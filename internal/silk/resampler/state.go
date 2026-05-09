// Package resampler implements the SILK sample-rate converter from
// vendor/silk/src/SKP_Silk_resampler*.c.
package resampler

// MAX_FIR_ORDER and MAX_IIR_ORDER mirror the same C constants from
// SKP_Silk_resampler_structs.h.
const (
	MaxFIROrder = 16
	MaxIIROrder = 6
)

// State mirrors SKP_Silk_resampler_state_struct from
// vendor/silk/src/SKP_Silk_resampler_structs.h.
//
// resamplerFunction and up2Function are dispatch hooks chosen by Init.
// Coefs is a slice into one of the rom tables (or nil).
type State struct {
	SIIR [MaxIIROrder]int32

	// SFIR carries the FIR delay line. The C source uses one void*-style
	// memory region as either int32 (down_FIR path) or int16 (IIR_FIR path,
	// via byte-level memcpy). We split into two typed fields here to avoid
	// endian-sensitive aliasing — Init wires up exactly one based on the
	// chosen resampler path.
	SFIR    [MaxFIROrder]int32
	SFIRInt [12]int16

	SDown2 [2]int32

	resamplerFunction func(s *State, out []int16, in []int16, inLen int32)
	up2Function       func(s []int32, out []int16, in []int16, inLen int32)

	BatchSize   int32
	InvRatioQ16 int32
	FIRFracs    int32
	Input2x     int32
	Coefs       []int16

	// Above-48 kHz extension.
	SDownPre [2]int32
	SUpPost  [2]int32

	downPreFunction func(s []int32, out []int16, in []int16, inLen int32)
	upPostFunction  func(s []int32, out []int16, in []int16, inLen int32)

	BatchSizePrePost int32
	RatioQ16         int32
	NPreDownsamplers int32
	NPostUpsamplers  int32

	MagicNumber int32
}

const stateMagic = int32(123456789)
