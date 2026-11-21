package cli

import (
	"mem/internal/env"
	"mem/internal/eval"
)

func cmdEval(e env.Env, args []string) int {
	return eval.Run(args, e)
}
