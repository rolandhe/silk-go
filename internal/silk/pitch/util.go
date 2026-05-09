package pitch

import "unsafe"

// unsafeInt32SliceAsInt16 reinterprets an int32 backing array as int16 to
// match the C source's `(SKP_int16*)scratch_mem` punning. Length is doubled.
func unsafeInt32SliceAsInt16(s []int32) []int16 {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*int16)(unsafe.Pointer(&s[0])), len(s)*2)
}
