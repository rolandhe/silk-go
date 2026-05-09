// silk-go is a SILK v3 codec CLI: encode PCM/WAV → .silk, decode .silk →
// PCM/WAV. Mirrors the rust-silk CLI surface so it can drop in as a
// replacement.
package main

import (
	"fmt"
	"os"

	"github.com/rolandhe/silk-go/internal/silk/dec"
	"github.com/rolandhe/silk-go/pkg/silkcli"
)

func main() {
	if len(os.Args) < 2 {
		silkcli.PrintUsage(os.Stderr)
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "encode":
		ea, err := silkcli.ParseEncodeArgs(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := silkcli.Encode(ea); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "decode":
		da, err := silkcli.ParseDecodeArgs(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := silkcli.Decode(da); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "--version":
		fmt.Printf("silk-sdk %s\n", dec.GetVersion())
	case "-h", "--help":
		silkcli.PrintUsage(os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		silkcli.PrintUsage(os.Stderr)
		os.Exit(1)
	}
}
