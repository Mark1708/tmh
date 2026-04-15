package main

import (
	"fmt"
	"os"

	"git.mark1708.ru/me/tmh/cmd/tmh/cmd"
)

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	root := cmd.NewRoot(Version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tmh:", err)
		os.Exit(1)
	}
}
