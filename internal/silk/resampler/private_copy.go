package resampler

// PrivateCopy — SKP_Silk_resampler_private_copy.
//
// Identity passthrough used when input and output rates match.
//
// Translated from vendor/silk/src/SKP_Silk_resampler_private_copy.c.
func PrivateCopy(s *State, out []int16, in []int16, inLen int32) {
	copy(out[:inLen], in[:inLen])
}
