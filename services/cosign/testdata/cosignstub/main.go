// Command cosignstub is a deterministic stand-in for the real cosign CLI,
// used by services/cosign tests so the suite needs no network, no keys, and
// no cosign install. It implements just enough of the CLI surface the
// Provider shells: version, sign-blob, verify-blob, attest-blob,
// verify-blob-attestation.
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
	case "attest-blob":
		runAttest(os.Args[2:])
	case "verify-blob-attestation":
		runVerifyAttest(os.Args[2:])
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
	bundleOut := flag(args, "--bundle")
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
	if key == "NOMARKER" {
		// Exit 0 with no verification marker: exercises the caller's
		// never-silent-success path.
		fmt.Println("done")
		return
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

func runAttest(args []string) {
	key := flag(args, "--key")
	predicate := flag(args, "--predicate")
	out := flag(args, "--bundle")
	if out == "" {
		out = flag(args, "--output-attestation")
	}
	blob := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			blob = a
		}
	}
	if predicate == "" || out == "" || blob == "" {
		fmt.Fprintln(os.Stderr, "attest-blob: predicate, bundle and blob are required")
		os.Exit(2)
	}
	if key == "MISSING" {
		fmt.Fprintln(os.Stderr, "attest-blob: no such key")
		os.Exit(1)
	}
	data, err := os.ReadFile(blob)
	if err != nil {
		fmt.Fprintln(os.Stderr, "attest-blob: "+err.Error())
		os.Exit(2)
	}
	if _, err := os.ReadFile(predicate); err != nil {
		fmt.Fprintln(os.Stderr, "attest-blob: "+err.Error())
		os.Exit(2)
	}
	// Empty key = keyless (Fulcio/OIDC) path: the real CLI would open a
	// browser here; the stub just records the envelope deterministically.
	if err := os.WriteFile(out, []byte("STUBATTEST:"+string(data)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "attest-blob: "+err.Error())
		os.Exit(1)
	}
	fmt.Println("Attestation created for " + blob)
}

func runVerifyAttest(args []string) {
	key := flag(args, "--key")
	env := flag(args, "--bundle")
	if env == "" {
		env = flag(args, "--signature")
	}
	identity := flag(args, "--certificate-identity")
	issuer := flag(args, "--certificate-oidc-issuer")
	blob := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			blob = a
		}
	}
	if env == "" || blob == "" {
		fmt.Fprintln(os.Stderr, "verify-blob-attestation: bundle and blob are required")
		os.Exit(2)
	}
	if key == "" && (identity == "" || issuer == "") {
		fmt.Fprintln(os.Stderr, "verify-blob-attestation: key or certificate-identity plus certificate-oidc-issuer is required")
		os.Exit(2)
	}
	if key == "WRONG" {
		fmt.Fprintln(os.Stderr, "Error: attestation verification failed: no matching key")
		os.Exit(1)
	}
	if key == "NOMARKER" {
		fmt.Println("done")
		return
	}
	raw, err := os.ReadFile(env)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify-blob-attestation: "+err.Error())
		os.Exit(1)
	}
	if !strings.HasPrefix(string(raw), "STUBATTEST:") {
		fmt.Fprintln(os.Stderr, "Error: attestation verification failed")
		os.Exit(1)
	}
	fmt.Println("Verified OK")
}
