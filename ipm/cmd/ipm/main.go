package main

import (
	"fmt"
	"ipm/pkg/cli"
	"os"
)

func main() {
    rootCmd := cli.NewRootCmd()
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}