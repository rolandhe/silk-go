package silk

// Version returned by GetVersion. The C SDK reported "1.0.9"; we keep that
// for parity so any downstream consumer expecting a numeric version still
// parses sensibly. The Go port adds a "+go" suffix.
const Version = "1.0.9+go"

// GetVersion returns the SILK SDK version string (mirrors SKP_Silk_SDK_get_version).
func GetVersion() string { return Version }
