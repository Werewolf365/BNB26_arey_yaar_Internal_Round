// Command quorum is the primary developer-facing product.
package main

import (
	"os"

	"github.com/quorum/quorum/apps/cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
