package main

import (
	"os"

	"mem/internal/cli"
	"mem/internal/env"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], env.System()))
}
