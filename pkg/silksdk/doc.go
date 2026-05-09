// Package silksdk is the in-process SDK for the SILK v3 audio codec
// implemented in github.com/rolandhe/silk-go. It exposes Encode and Decode
// as direct Go function calls — no subprocess, no file paths, no progress
// printing — so callers can embed the codec with io.Reader / io.Writer and
// pure-memory pipelines (e.g. *bytes.Buffer).
//
// Output bytes are byte-identical to what the cmd/silk-go CLI produces with
// equivalent flags, which in turn is byte-identical to rust-silk's vendored
// SILK reference implementation across the verified test matrix.
//
// Companion package: pkg/silkcli — the CLI-flavoured wrapper that adds
// file I/O, progress prints, and stats-to-stderr. The SDK has none of that;
// callers wire stats via *EncodeStats / *DecodeMetrics pointers.
//
// Quick start — round-trip in memory:
//
//	pcm := makePCM()                    // []byte, mono s16le @ 16 kHz
//	var silkBuf bytes.Buffer
//	if err := silksdk.Encode(silksdk.EncodeOptions{
//	    Input:      bytes.NewReader(pcm),
//	    Output:     &silkBuf,
//	    SampleRate: 16000,
//	    BitRate:    25000,
//	    Complexity: 2,
//	}); err != nil { ... }
//
//	var pcmBuf bytes.Buffer
//	if err := silksdk.Decode(silksdk.DecodeOptions{
//	    Input:      &silkBuf,
//	    Output:     &pcmBuf,
//	    SampleRate: 16000,
//	}); err != nil { ... }
package silksdk
