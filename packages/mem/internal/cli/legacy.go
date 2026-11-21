package cli

import (
	"mem/internal/env"
	"mem/internal/legacy"
)

func cmdImportLegacy(e env.Env, args []string) int {
	return legacy.Run(args, e)
}
