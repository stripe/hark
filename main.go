package main

import (
	"os"

	"github.com/stripe/hark/cmd"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cmd.Execute(version))
}
