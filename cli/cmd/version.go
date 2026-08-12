package cmd

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// Version is stamped at release time via
// -ldflags "-X github.com/daforester/goleo/cli/cmd.Version=<v>"
// (see cli/npm/build-platform-packages.js). It stays "dev" for unstamped builds,
// where resolveVersion falls back to the module version baked into the binary
// (e.g. from `go install github.com/daforester/goleo/cli/goleo@v0.1.2`).
var Version = "dev"

func resolveVersion() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return Version
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Goleo",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Goleo v%s\n", resolveVersion())
	},
}
