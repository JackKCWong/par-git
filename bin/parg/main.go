package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "parg",
	Short: "parg is a parallel git operations tool",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}