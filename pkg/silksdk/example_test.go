package silksdk_test

import (
	"bytes"
	"fmt"

	"github.com/rolandhe/silk-go/pkg/silksdk"
)

// ExampleEncode shows the simplest in-memory encode call.
func ExampleEncode() {
	pcm := make([]byte, 16000*2) // 1 second of silence at 16 kHz mono s16le
	var silkBuf bytes.Buffer

	err := silksdk.Encode(silksdk.EncodeOptions{
		Input:      bytes.NewReader(pcm),
		Output:     &silkBuf,
		SampleRate: 16000,
		BitRate:    25000,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(silkBuf.Bytes()[:9])) // header magic
	// Output: #!SILK_V3
}

// ExampleDecode shows decoding a SILK byte stream into PCM.
func ExampleDecode() {
	// Build a tiny silk stream by encoding 100ms of silence first.
	pcm := make([]byte, 16000*2*100/1000)
	var silkBuf bytes.Buffer
	if err := silksdk.Encode(silksdk.EncodeOptions{
		Input:      bytes.NewReader(pcm),
		Output:     &silkBuf,
		SampleRate: 16000,
	}); err != nil {
		panic(err)
	}

	var pcmOut bytes.Buffer
	if err := silksdk.Decode(silksdk.DecodeOptions{
		Input:      &silkBuf,
		Output:     &pcmOut,
		SampleRate: 16000,
	}); err != nil {
		panic(err)
	}
	fmt.Printf("decoded %d PCM bytes\n", pcmOut.Len())
	// Output: decoded 3200 PCM bytes
}
