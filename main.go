// Package main is the alternative entry point for cli-replay with record support.
package main

import (
	"fmt"
	"os"

	"github.com/cli-replay/cli-replay/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
