package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "goleo",
	Short: "Goleo - Build cross-platform apps with Go and web technologies",
	Long: `Goleo is a framework for building cross-platform desktop and mobile
applications using Go for the backend and web technologies (HTML, CSS, JS,
Vue, React, etc.) for the frontend.

Supports Windows, Linux, macOS, Android, and iOS from a single codebase.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(devCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(emulateCmd)
	rootCmd.AddCommand(versionCmd)
}
