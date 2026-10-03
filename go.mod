module github.com/quorum/quorum

go 1.27

// Pinned toolchain: Quorum validates against the currently supported Go
// release line (Go 1.27.x as verified Oct 2026 via https://go.dev/doc/devel/release).
// CI verifies `go version` matches this line. Do NOT silently upgrade;
// bump deliberately with go.mod + CI + docs update.
toolchain go1.27.1

require (
	github.com/jackc/pgx/v5 v5.11.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/crypto v0.57.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)
