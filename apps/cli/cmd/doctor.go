package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/spf13/cobra"
)

// windowsFallback probes standard install paths when PATH is stale.
var windowsFallback = map[string]string{
	"go": "C:\\Program Files\\Go\\bin\\go.exe",
}

func isWindows() bool { return runtime.GOOS == "windows" }

func newDoctorCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check environment toolchains and connectivity (never installs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			api, _ := cmd.Flags().GetString("api")
			bad := 0
			check := func(name, hint string, ok bool, detail string) {
				status := "PASS"
				if !ok {
					status = "MISS"
					bad++
				}
				if !useQuiet(cmd) && !useJSON(cmd) {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s %s\n", status, name, detail)
					if !ok && hint != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "       -> %s\n", hint)
					}
				}
			}
			tool := func(name, hint string, args ...string) {
				out, err := exec.Command(name, args...).Output()
				if err != nil && isWindows() {
					// Fresh installs often miss PATH in older sessions; probe the
					// standard location before reporting MISSING.
					if alt, ok := windowsFallback[name]; ok {
						out, err = exec.Command(alt, args...).Output()
					}
				}
				if err != nil {
					check(name, hint, false, "MISSING")
					return
				}
				first := string(out)
				if len(first) > 80 {
					first = first[:80]
				}
				check(name, hint, true, firstLine(first))
			}
			tool("go", "Install: https://go.dev/dl/ (pinned go1.27.x, see go.mod)", "version")
			tool("node", "Install Node 20+ for the web dashboard", "--version")
			tool("docker", "Install + start Docker Desktop", "--version")
			optionalTool(cmd, "cosign", "Optional until Sigstore phase: https://docs.sigstore.dev/cosign/installation/", "version")
			tool("git", "Install git", "--version")
			if err := exec.Command("docker", "info").Run(); err != nil {
				check("docker-daemon", "Start Docker Desktop", false, "NOT running")
			} else {
				check("docker-daemon", "", true, "running")
			}
			if foundryImagePresent() {
				check("foundry-image", "", true, "ghcr.io/foundry-rs/foundry present (forge/cast via Docker)")
			} else {
				check("foundry-image", "docker pull ghcr.io/foundry-rs/foundry:latest (or use scripts/forge-docker.ps1)", false, "absent")
			}
			if api != "" {
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Get(api)
				if err != nil {
					check("api", "start the backend API", false, err.Error())
				} else {
					resp.Body.Close()
					check("api", "", true, api)
				}
			}
			for _, env := range []string{"QUORUM_ANCHOR_CONTRACT", "QUORUM_RPC_URL"} {
				if os.Getenv(env) == "" {
					if !useQuiet(cmd) && !useJSON(cmd) {
						fmt.Fprintf(cmd.OutOrStdout(), "INFO  %s unset (optional)\n", env)
					}
				}
			}
			f, err := os.CreateTemp(os.TempDir(), "quorum-doc-*")
			if err != nil {
				check("fs-write", "check temp dir permissions", false, err.Error())
			} else {
				f.Close()
				os.Remove(f.Name())
				check("fs-write", "", true, "temp dir writable")
			}
			if bad > 0 {
				return &exitErr{code: exitcodes.Operational}
			}
			return nil
		},
	}
	c.Flags().String("api", "", "optional API base URL to probe (e.g. http://localhost:8080/health)")
	return c
}

func optionalTool(cmd *cobra.Command, name, hint string, args ...string) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		if !useQuiet(cmd) && !useJSON(cmd) {
			fmt.Fprintf(cmd.OutOrStdout(), "INFO  %s absent (optional): %s\n", name, hint)
		}
		return
	}
	first := string(out)
	if len(first) > 80 {
		first = first[:80]
	}
	if !useQuiet(cmd) && !useJSON(cmd) {
		fmt.Fprintf(cmd.OutOrStdout(), "PASS  %s %s\n", name, firstLine(first))
	}
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

func foundryImagePresent() bool {
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		return false
	}
	for _, line := range splitLines(string(out)) {
		if line == "ghcr.io/foundry-rs/foundry:latest" {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else if r != '\r' {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
