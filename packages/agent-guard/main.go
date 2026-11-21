package main

import (
	"os"

	"agent-guard/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.System()))
}
