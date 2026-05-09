package silk

// Error codes from vendor/silk/interface/SKP_Silk_errors.h. Returned as int32
// by the public API, with sentinel Error wrappers exposed for callers that
// prefer error values.

const (
	NoError int32 = 0

	// Encoder errors.
	EncInputInvalidNoOfSamples    int32 = -1
	EncFsNotSupported             int32 = -2
	EncPacketSizeNotSupported     int32 = -3
	EncPayloadBufTooShort         int32 = -4
	EncInvalidLossRate            int32 = -5
	EncInvalidComplexitySetting   int32 = -6
	EncInvalidInbandFecSetting    int32 = -7
	EncInvalidDtxSetting          int32 = -8
	EncInternalError              int32 = -9

	// Decoder errors.
	DecInvalidSamplingFrequency int32 = -10
	DecPayloadTooLarge          int32 = -11
	DecPayloadError             int32 = -12

	// Range coder errors (vendor/silk/src/SKP_Silk_define.h).
	RangeCoderWriteBeyondBuffer  int32 = -1
	RangeCoderCDFOutOfRange      int32 = -2
	RangeCoderNormalizationFail  int32 = -3
	RangeCoderZeroIntervalWidth  int32 = -4
	RangeCoderDecoderCheckFailed int32 = -5
	RangeCoderReadBeyondBuffer   int32 = -6
	RangeCoderIllegalSampling    int32 = -7
	RangeCoderDecPayloadTooLong  int32 = -8
)

// Error implements the error interface for an int32 error code so callers can
// wrap returns directly without a translation table at every call site.
type Error int32

func (e Error) Error() string {
	switch int32(e) {
	case 0:
		return "silk: no error"
	case -1:
		return "silk: input length is not a multiple of 10ms or exceeds packet length"
	case -2:
		return "silk: sampling frequency not 8000/12000/16000/24000 Hz"
	case -3:
		return "silk: packet size not 20/40/60/80/100 ms"
	case -4:
		return "silk: payload buffer too short"
	case -5:
		return "silk: loss rate must be in [0,100]"
	case -6:
		return "silk: complexity must be 0/1/2"
	case -7:
		return "silk: inband FEC must be 0/1"
	case -8:
		return "silk: DTX must be 0/1"
	case -9:
		return "silk: internal encoder error"
	case -10:
		return "silk: output sample rate below internal decoded rate"
	case -11:
		return "silk: payload exceeds 1024 bytes"
	case -12:
		return "silk: payload bit error"
	default:
		return "silk: unknown error"
	}
}
