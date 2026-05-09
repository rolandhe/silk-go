package resampler

import (
	"github.com/rolandhe/silk-go/internal/silk/fix"
	"github.com/rolandhe/silk-go/internal/silk/tables"
)

// gcd — Euclidean GCD used by Init's batch-size logic.
func gcd(a, b int32) int32 {
	for b > 0 {
		tmp := a - b*(a/b)
		a = b
		b = tmp
	}
	return a
}

// Init — SKP_Silk_resampler_init.
//
// Set up `s` to convert from FsHzIn to FsHzOut. Returns -1 for invalid
// rates, 0 on success.
//
// Translated from vendor/silk/src/SKP_Silk_resampler.c.
func Init(s *State, FsHzIn, FsHzOut int32) int32 {
	*s = State{}

	if FsHzIn < 8000 || FsHzIn > 192000 || FsHzOut < 8000 || FsHzOut > 192000 {
		return -1
	}

	// Pre-down/post-up for >48 kHz endpoints.
	switch {
	case FsHzIn > 96000:
		s.NPreDownsamplers = 2
		s.downPreFunction = func(state []int32, out []int16, in []int16, inLen int32) {
			PrivateDown4(state, out, in, inLen)
		}
	case FsHzIn > 48000:
		s.NPreDownsamplers = 1
		s.downPreFunction = func(state []int32, out []int16, in []int16, inLen int32) {
			Down2(state, out, in, inLen)
		}
	}
	switch {
	case FsHzOut > 96000:
		s.NPostUpsamplers = 2
		s.upPostFunction = func(state []int32, out []int16, in []int16, inLen int32) {
			PrivateUp4(state, out, in, inLen)
		}
	case FsHzOut > 48000:
		s.NPostUpsamplers = 1
		s.upPostFunction = func(state []int32, out []int16, in []int16, inLen int32) {
			Up2(state, out, in, inLen)
		}
	}

	if s.NPreDownsamplers+s.NPostUpsamplers > 0 {
		s.RatioQ16 = fix.LShift32(fix.Div32(fix.LShift32(FsHzOut, 13), FsHzIn), 3)
		for fix.SmulWW(s.RatioQ16, FsHzIn) < FsHzOut {
			s.RatioQ16++
		}
		s.BatchSizePrePost = fix.Div32By16(FsHzIn, 100)
		FsHzIn = fix.RShift32(FsHzIn, s.NPreDownsamplers)
		FsHzOut = fix.RShift32(FsHzOut, s.NPostUpsamplers)
	}

	// Batch size — try 10 ms frames, fall back to GCD-derived size.
	s.BatchSize = fix.Div32By16(FsHzIn, 100)
	if fix.Mul(s.BatchSize, 100) != FsHzIn || FsHzIn%100 != 0 {
		cycleLen := fix.Div32(FsHzIn, gcd(FsHzIn, FsHzOut))
		cyclesPerBatch := fix.Div32(ResamplerMaxBatchSizeIn, cycleLen)
		if cyclesPerBatch == 0 {
			s.BatchSize = ResamplerMaxBatchSizeIn
		} else {
			s.BatchSize = fix.Mul(cyclesPerBatch, cycleLen)
		}
	}

	var up2, down2 int32
	switch {
	case FsHzOut > FsHzIn:
		// Upsample.
		if FsHzOut == fix.Mul(FsHzIn, 2) {
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateUp2HQWrapper(st, out, in, inLen)
			}
		} else {
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateIIRFIR(st, out, in, inLen)
			}
			up2 = 1
			if FsHzIn > 24000 {
				s.up2Function = func(state []int32, out []int16, in []int16, inLen int32) {
					Up2(state, out, in, inLen)
				}
			} else {
				s.up2Function = func(state []int32, out []int16, in []int16, inLen int32) {
					PrivateUp2HQ(state, out, in, inLen)
				}
			}
		}
	case FsHzOut < FsHzIn:
		// Downsample — try the rational-ratio shortcuts before falling back.
		switch {
		case fix.Mul(FsHzOut, 4) == fix.Mul(FsHzIn, 3): // 3:4
			s.FIRFracs = 3
			s.Coefs = tables.Resampler_3_4_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateDownFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 3) == fix.Mul(FsHzIn, 2): // 2:3
			s.FIRFracs = 2
			s.Coefs = tables.Resampler_2_3_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateDownFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 2) == FsHzIn: // 1:2
			s.FIRFracs = 1
			s.Coefs = tables.Resampler_1_2_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateDownFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 8) == fix.Mul(FsHzIn, 3): // 3:8
			s.FIRFracs = 3
			s.Coefs = tables.Resampler_3_8_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateDownFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 3) == FsHzIn: // 1:3
			s.FIRFracs = 1
			s.Coefs = tables.Resampler_1_3_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateDownFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 4) == FsHzIn: // 1:4 (down2 + 1:2 FIR)
			s.FIRFracs = 1
			down2 = 1
			s.Coefs = tables.Resampler_1_2_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateDownFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 6) == FsHzIn: // 1:6 (down2 + 1:3 FIR)
			s.FIRFracs = 1
			down2 = 1
			s.Coefs = tables.Resampler_1_3_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateDownFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 441) == fix.Mul(FsHzIn, 80):
			s.Coefs = tables.Resampler_80_441_ARMA4_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateIIRFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 441) == fix.Mul(FsHzIn, 120):
			s.Coefs = tables.Resampler_120_441_ARMA4_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateIIRFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 441) == fix.Mul(FsHzIn, 160):
			s.Coefs = tables.Resampler_160_441_ARMA4_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateIIRFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 441) == fix.Mul(FsHzIn, 240):
			s.Coefs = tables.Resampler_240_441_ARMA4_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateIIRFIR(st, out, in, inLen)
			}
		case fix.Mul(FsHzOut, 441) == fix.Mul(FsHzIn, 320):
			s.Coefs = tables.Resampler_320_441_ARMA4_COEFS[:]
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateIIRFIR(st, out, in, inLen)
			}
		default:
			s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
				PrivateIIRFIR(st, out, in, inLen)
			}
			up2 = 1
			if FsHzIn > 24000 {
				s.up2Function = func(state []int32, out []int16, in []int16, inLen int32) {
					Up2(state, out, in, inLen)
				}
			} else {
				s.up2Function = func(state []int32, out []int16, in []int16, inLen int32) {
					PrivateUp2HQ(state, out, in, inLen)
				}
			}
		}
	default:
		// Equal — copy.
		s.resamplerFunction = func(st *State, out []int16, in []int16, inLen int32) {
			PrivateCopy(st, out, in, inLen)
		}
	}

	s.Input2x = up2 | down2
	s.InvRatioQ16 = fix.LShift32(fix.Div32(fix.LShift32(FsHzIn, 14+up2-down2), FsHzOut), 2)
	for fix.SmulWW(s.InvRatioQ16, fix.LShift32(FsHzOut, down2)) < fix.LShift32(FsHzIn, up2) {
		s.InvRatioQ16++
	}

	s.MagicNumber = stateMagic
	return 0
}

// Clear — SKP_Silk_resampler_clear. Zero filter states without disturbing
// the configured ratio/dispatch.
func Clear(s *State) int32 {
	s.SDown2 = [2]int32{}
	s.SIIR = [MaxIIROrder]int32{}
	s.SFIR = [MaxFIROrder]int32{}
	s.SFIRInt = [12]int16{}
	s.SDownPre = [2]int32{}
	s.SUpPost = [2]int32{}
	return 0
}

// Process — SKP_Silk_resampler.
//
// Convert `in` to `out` via the configured path. Returns -1 if `s` was
// not initialized.
//
// Translated from vendor/silk/src/SKP_Silk_resampler.c.
func Process(s *State, out []int16, in []int16, inLen int32) int32 {
	if s.MagicNumber != stateMagic {
		return -1
	}

	if s.NPreDownsamplers+s.NPostUpsamplers > 0 {
		var inBuf, outBuf [480]int16
		inOff := int32(0)
		outOff := int32(0)
		for inLen > 0 {
			nSamplesIn := inLen
			if nSamplesIn > s.BatchSizePrePost {
				nSamplesIn = s.BatchSizePrePost
			}
			nSamplesOut := fix.SmulWB(s.RatioQ16, nSamplesIn)

			midLen := fix.RShift32(nSamplesIn, s.NPreDownsamplers)
			if s.NPreDownsamplers > 0 {
				s.downPreFunction(s.SDownPre[:], inBuf[:midLen], in[inOff:inOff+nSamplesIn], nSamplesIn)
				if s.NPostUpsamplers > 0 {
					s.resamplerFunction(s, outBuf[:fix.RShift32(nSamplesOut, s.NPostUpsamplers)],
						inBuf[:midLen], midLen)
					s.upPostFunction(s.SUpPost[:], out[outOff:outOff+nSamplesOut],
						outBuf[:fix.RShift32(nSamplesOut, s.NPostUpsamplers)],
						fix.RShift32(nSamplesOut, s.NPostUpsamplers))
				} else {
					s.resamplerFunction(s, out[outOff:outOff+nSamplesOut], inBuf[:midLen], midLen)
				}
			} else {
				s.resamplerFunction(s, outBuf[:fix.RShift32(nSamplesOut, s.NPostUpsamplers)],
					in[inOff:inOff+nSamplesIn], midLen)
				s.upPostFunction(s.SUpPost[:], out[outOff:outOff+nSamplesOut],
					outBuf[:fix.RShift32(nSamplesOut, s.NPostUpsamplers)],
					fix.RShift32(nSamplesOut, s.NPostUpsamplers))
			}

			inOff += nSamplesIn
			outOff += nSamplesOut
			inLen -= nSamplesIn
		}
	} else {
		s.resamplerFunction(s, out, in, inLen)
	}
	return 0
}
