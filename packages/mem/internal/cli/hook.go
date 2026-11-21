package cli

import (
	"mem/internal/env"
	"mem/internal/hook"
)

func cmdHook(e env.Env, args []string) int {
	return hook.Run(args, e)
}
