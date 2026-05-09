package main

import (
	"os"

	"github.com/dollarlint/dollarlint/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
