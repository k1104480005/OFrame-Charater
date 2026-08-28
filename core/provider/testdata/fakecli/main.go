// Command fakecli is a deterministic test double for the CLI provider tests
// (align-framebaker-providers tasks 3.2/3.3). It lives under testdata/ so the
// Go tool ignores it during normal builds; the tests compile it with
// `go build -o … ./testdata/fakecli` once per run.
//
// Behavior is selected with the FAKECLI_MODE environment variable:
//
//	ok (default)  writes a valid PNG to the --output value and exits 0
//	fail-exit     writes a message to stderr and exits 3
//	no-output     exits 0 WITHOUT producing the output file
//	bad-format    writes plain text bytes to the output file
//	empty-output  writes a zero-byte output file
//
// When FAKECLI_LOG is set, one JSON line describing the received argv and the
// sizes of every --ref file is appended to that path, letting tests assert
// that spaces and special characters stayed inside single argument elements.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	mode := os.Getenv("FAKECLI_MODE")
	logPath := os.Getenv("FAKECLI_LOG")
	args := os.Args[1:]

	if logPath != "" {
		entry := map[string]any{"args": args, "refSizes": refSizes(args)}
		if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			if data, err := json.Marshal(entry); err == nil {
				f.Write(data)
				f.WriteString("\n")
			}
			f.Close()
		}
	}

	out := ""
	for i, a := range args {
		if a == "--output" && i+1 < len(args) {
			out = args[i+1]
		}
	}

	// A minimal but structurally valid PNG header (magic + IHDR chunk start).
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R'}

	switch mode {
	case "fail-exit":
		fmt.Fprintln(os.Stderr, "fakecli: deliberate failure for tests")
		os.Exit(3)
	case "no-output":
		return
	case "bad-format":
		if out != "" {
			_ = os.WriteFile(out, []byte("definitely not an image"), 0o600)
		}
		return
	case "empty-output":
		if out != "" {
			_ = os.WriteFile(out, nil, 0o600)
		}
		return
	default: // ok
		if out == "" {
			fmt.Fprintln(os.Stderr, "fakecli: no --output value received")
			os.Exit(2)
		}
		if err := os.WriteFile(out, png, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "fakecli: write output: %v\n", err)
			os.Exit(4)
		}
	}
}

// refSizes stats every --ref <path> pair so tests can verify the reference
// bytes actually reached the adapter's temp files.
func refSizes(args []string) map[string]int64 {
	sizes := map[string]int64{}
	for i, a := range args {
		if a == "--ref" && i+1 < len(args) {
			if info, err := os.Stat(args[i+1]); err == nil {
				sizes[args[i+1]] = info.Size()
			} else {
				sizes[args[i+1]+" (error)"] = -1
			}
		}
	}
	return sizes
}
