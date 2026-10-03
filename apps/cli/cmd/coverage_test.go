package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/quorum/quorum/services/sigstore"
	"github.com/spf13/cobra"
)

func TestPureHelpers(t *testing.T) {
	if got := firstLine("a\nb\nc"); got != "a" {
		t.Fatalf("firstLine: %q", got)
	}
	if got := firstLine("single"); got != "single" {
		t.Fatalf("firstLine no newline: %q", got)
	}
	if got := splitLines("a\r\nb\nc"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("splitLines CRLF: %q", got)
	}
	if got := splitLines(""); len(got) != 0 {
		t.Fatalf("splitLines empty: %q", got)
	}
	if isWindows() != (runtime.GOOS == "windows") {
		t.Fatal("isWindows must track runtime")
	}
	if orDefault("", "d") != "d" || orDefault("v", "d") != "v" {
		t.Fatal("orDefault")
	}
}

func TestOptionalTool(t *testing.T) {
	newBufCmd := func() (*cobra.Command, *strings.Builder) {
		c := &cobra.Command{}
		c.Flags().Bool("json", false, "")
		c.Flags().Bool("quiet", false, "")
		var b strings.Builder
		c.SetOut(&b)
		return c, &b
	}
	c, out := newBufCmd()
	optionalTool(c, "tool-definitely-missing", "hint-hint", "version")
	if !strings.Contains(out.String(), "absent (optional)") {
		t.Fatalf("missing optional must be INFO: %q", out.String())
	}
	c2, out2 := newBufCmd()
	optionalTool(c2, "go", "hint", "version")
	if !strings.Contains(out2.String(), "PASS") {
		t.Fatalf("present optional must be PASS: %q", out2.String())
	}
}

func TestDoctorAPIProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`ok`))
	}))
	defer srv.Close()
	if code, out, _ := execute("doctor", "--api", srv.URL); !strings.Contains(out, "api") {
		t.Fatalf("doctor must probe api: %d %s", code, out)
	}
	if code, out, _ := execute("doctor", "--api", "http://127.0.0.1:9"); !strings.Contains(out, "api") {
		t.Fatalf("doctor must report dead api: %d %s", code, out)
	} else if code != 4 {
		t.Fatalf("dead api must exit 4: %d", code)
	}
}

func TestVersionModes(t *testing.T) {
	if code, out, _ := execute("version", "--json"); code != 0 || !strings.Contains(out, "version") {
		t.Fatalf("version json: %d %s", code, out)
	}
	if code, out, _ := execute("version", "--quiet"); code != 0 || out != "" {
		t.Fatalf("version quiet: %d %q", code, out)
	}
}

func TestInspectEdges(t *testing.T) {
	if code, _, _ := execute("inspect"); code != 5 {
		t.Fatalf("inspect without args must exit 5, got %d", code)
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{oops"), 0o600)
	if code, _, _ := execute("inspect", bad); code != 5 {
		t.Fatalf("inspect malformed must exit 5, got %d", code)
	}
	rej := filepath.Join(dir, "rej.json")
	os.WriteFile(rej, []byte(`{"decision":"REJECTED","artifact":"a","source":"s"}`), 0o600)
	if code, _, _ := execute("inspect", rej); code != 1 {
		t.Fatalf("inspect REJECTED must exit 1, got %d", code)
	}
}

func TestEntryPoints(t *testing.T) {
	// NOTE: Execute() itself reads os.Args and is covered via the built
	// binary, not unit tests (test-binary flags would leak into parsing).
	if (&exitErr{code: 3}).Error() != "exit 3" {
		t.Fatal("exitErr must render its code")
	}
	if got := builderOf(sigstore.Statement{}); got != "" {
		t.Fatalf("empty statement must have no builder, got %q", got)
	}
	if got := cosignExit("MALFORMED_EVIDENCE"); got != 5 {
		t.Fatalf("unexpected cosign state must be invalid input, got %d", got)
	}
	// emit with an unmarshalable value under --json surfaces the error.
	c := &cobra.Command{}
	c.Flags().Bool("json", false, "")
	c.Flags().Bool("quiet", false, "")
	_ = c.Flags().Set("json", "true")
	if err := emit(c, "", func() {}); err == nil {
		t.Fatal("unmarshalable json value must error")
	}
}

func TestAttestErrorPaths(t *testing.T) {
	dir := t.TempDir()
	// genkey to an unwritable path.
	if code, _, _ := execute("attest", "genkey", "--private", dir, "--public", filepath.Join(dir, "p.pem")); code != 4 {
		t.Fatalf("genkey unwritable must exit 4, got %d", code)
	}
	// sign with missing artifact file.
	priv := filepath.Join(dir, "k.pem")
	pub := filepath.Join(dir, "k.pub")
	if code, _, _ := execute("attest", "genkey", "--private", priv, "--public", pub); code != 0 {
		t.Fatalf("genkey: %d", code)
	}
	if code, _, _ := execute("attest", "sign", "--artifact", filepath.Join(dir, "nope.bin"),
		"--commit", "c", "--builder", "b", "--key", priv); code != 5 {
		t.Fatalf("sign missing artifact must exit 5, got %d", code)
	}
	// sign with a garbage key file.
	artifact := filepath.Join(dir, "a.bin")
	os.WriteFile(artifact, []byte("data"), 0o600)
	badkey := filepath.Join(dir, "bad.pem")
	os.WriteFile(badkey, []byte("nope"), 0o600)
	if code, _, _ := execute("attest", "sign", "--artifact", artifact,
		"--commit", "c", "--builder", "b", "--key", badkey); code != 5 {
		t.Fatalf("sign bad key must exit 5, got %d", code)
	}
}
