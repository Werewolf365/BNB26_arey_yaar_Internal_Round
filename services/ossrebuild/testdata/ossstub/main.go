// Command ossstub is a deterministic stand-in for the real oss-rebuild CLI,
// used by services/ossrebuild tests so Lookup paths need no network.
package main

import (
	"fmt"
	"os"
	"strings"
)

const digest = "9a28abb62774ae4e8edbe2dd4c49ffcd45a6a848952a5eccc6a49f3f0fc1e2f3"
const commit = "37dad4d356ca9e13f1c533ad6309631b397a2b6b"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--help" {
		fmt.Println("ossstub --help")
		return
	}
	// get <eco> <name> <version> --output=<kind>
	if len(os.Args) < 6 || os.Args[1] != "get" {
		fmt.Fprintln(os.Stderr, "usage: ossstub get <eco> <name> <version> --output=<kind>")
		os.Exit(2)
	}
	eco, name, ver, out := os.Args[2], os.Args[3], os.Args[4], os.Args[5]
	if eco == "pypi" && name == "absl-py" && ver == "2.0.0" && out == "--output=summary" {
		fmt.Println("Rebuild found!")
		fmt.Printf("Upstream target digest: {\"sha256\":\"%s\"}\n", digest)
		fmt.Printf("git checkout --force '%s'\n", commit)
		fmt.Println(`Rebuilt at: "2026-01-01T00:00:00Z"`)
		fmt.Println(`artifact is being inferred as "absl_py-2.0.0-py3-none-any.whl"`)
		return
	}
	if eco == "pypi" && name == "absl-py" && ver == "2.0.0" && out == "--output=payload" {
		fmt.Printf(`{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://slsa.dev/provenance/v1","subject":[{"digest":{"sha256":"%s"}}]}`, digest)
		fmt.Println()
		return
	}
	if eco == "pypi" && name == "absl-py" && ver == "2.0.0-badpayload" && out == "--output=payload" {
		fmt.Println(`{"nope":true}`)
		return
	}
	if strings.HasSuffix(ver, "bogus-quorum") {
		fmt.Fprintln(os.Stderr, "file does not exist")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "unexpected rebuild failure")
	os.Exit(1)
}
