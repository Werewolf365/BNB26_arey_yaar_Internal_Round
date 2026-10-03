// Command cosignstub is a deterministic stand-in for the real cosign CLI,
// used by services/cosign tests so the suite needs no network, no keys, and
// no cosign install. It implements just enough of the CLI surface the
// Provider shells: version, sign-blob, verify-blob.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cosignstub <command>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("cosignstub version 0.0.0-test")
		return
	case "sign-blob":
		runSign(os.Args[2:])
	case "verify-blob":
		runVerify(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "unknown command "+os.Args[1])
		os.Exit(2)
	}
}

func flag(args []string, name string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], name+"=") {
			return strings.TrimPrefix(args[i], name+"=")
		}
	}
	return ""
}

func runSign(args []string) {
	key := flag(args, "--key")
	sigOut := flag(args, "--output-signature")
	bundleOut := flag(args, "--output-bundle")
	blob := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			blob = a
		}
	}
	if key == "" || sigOut == "" || blob == "" {
		fmt.Fprintln(os.Stderr, "sign-blob: key, output-signature and blob are required")
		os.Exit(2)
	}
	if key == "MISSING" {
		fmt.Fprintln(os.Stderr, "sign-blob: no such key")
		os.Exit(1)
	}
	data, err := os.ReadFile(blob)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sign-blob: "+err.Error())
		os.Exit(2)
	}
	if err := os.WriteFile(sigOut, []byte("STUBSIG:"+string(data)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "sign-blob: "+err.Error())
		os.Exit(1)
	}
	if bundleOut != "" {
		_ = os.WriteFile(bundleOut, []byte(`{"stubBundle":true}`), 0o600)
	}
	fmt.Println("Signed " + blob)
}

func runVerify(args []string) {
	key := flag(args, "--key")
	sigPath := flag(args, "--signature")
	blob := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			blob = a
		}
	}
	if key == "" || sigPath == "" || blob == "" {
		fmt.Fprintln(os.Stderr, "verify-blob: key, signature and blob are required")
		os.Exit(2)
	}
	if key == "WRONG" {
		fmt.Fprintln(os.Stderr, "Error: signature verification failed: no matching key")
		os.Exit(1)
	}
	raw, err := os.ReadFile(sigPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify-blob: "+err.Error())
		os.Exit(1)
	}
	if !strings.HasPrefix(string(raw), "STUBSIG:") {
		fmt.Fprintln(os.Stderr, "Error: signature verification failed")
		os.Exit(1)
	}
	fmt.Println("Verified OK")
}
