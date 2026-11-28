package cli

import (
	"fmt"

	"mem/internal/env"
	"mem/internal/hook"
)

func cmdHook(e env.Env, args []string) int {
	return hook.Run(args, e)
}

func cmdHere(e env.Env, args []string) int {
	f := newFlags(&e, "here", "[--cwd DIR]", true)
	cwd := f.String("cwd", "", "directory to name the scopes of (default: the working directory)")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		return f.usageErr("unexpected arguments")
	}
	scopes := hook.LocalScopes(e, *cwd)
	if f.asJSON() {
		return fail(e, printJSON(e.Stdout, scopes))
	}
	for _, s := range scopes {
		_, _ = fmt.Fprintln(e.Stdout, s)
	}
	return 0
}
