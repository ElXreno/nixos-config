package checks

import (
	"strings"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

// gitCall splits a git invocation into its subcommand and the arguments that
// follow it, skipping the global options git accepts before the subcommand.
func gitCall(cmd shell.Command) (sub string, args []string, ok bool) {
	_, sub, args, ok = gitInvocation(cmd)
	return sub, args, ok
}

func gitInvocation(cmd shell.Command) (globals []string, sub string, args []string, ok bool) {
	if cmd.Head() != "git" {
		return nil, "", nil, false
	}
	all := cmd.Args()
	rest := all
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch rest[0] {
		case "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path":
			if len(rest) > 1 {
				rest = rest[2:]
			} else {
				rest = rest[1:]
			}
		default:
			rest = rest[1:]
		}
	}
	if len(rest) == 0 {
		return nil, "", nil, false
	}
	return all[:len(all)-len(rest)], rest[0], rest[1:], true
}

const addMsg = "Blocked: git add of the whole tree also stages the user's own uncommitted work and other sessions' changes. Stage the paths this task changed, git add <paths>, or commit them in one step with git commit -m <msg> -- <paths>. When the user asked to commit everything, name the paths explicitly."
const commitAllMsg = "Blocked: git commit -a sweeps every tracked modification into the commit, including the user's own and other sessions' work. Commit the paths this task changed: git commit -m <msg> -- <paths>."

func gitScope(cmd shell.Command, _ *Settings) *verdict.Verdict {
	sub, args, ok := gitCall(cmd)
	if !ok {
		return nil
	}
	switch sub {
	case "add":
		if contains(args, "-A", "--all", "-u", "--update", ".", "./", ":/", "*") || hasShortFlag(args, 'A') || hasShortFlag(args, 'u') {
			return deny(addMsg)
		}
	case "commit":
		if contains(args, "--all") || hasShortFlag(args, 'a') {
			return deny(commitAllMsg)
		}
	}
	return nil
}

const checkoutMsg = "A tree-wide checkout/restore throws away the user's uncommitted work. Yes only if the user asked for it; otherwise revert by path: git checkout -- <paths>."
const resetHardMsg = "git reset --hard destroys uncommitted work. Yes only if the user asked for it; otherwise git reset (mixed), git reset --soft, or revert specific paths."
const stashMsg = "git stash hides the user's dirty tree and is easy to lose. Yes only if the user asked for it; otherwise leave their changes in place and work around them."
const cleanMsg = "git clean deletes untracked files the user may still need. Yes only if the user asked for it; otherwise remove the specific paths this task created."

func gitDestructive(cmd shell.Command, _ *Settings) *verdict.Verdict {
	sub, args, ok := gitCall(cmd)
	if !ok {
		return nil
	}
	switch sub {
	case "checkout", "restore":
		if contains(args, ".", "./", ":/", "-A", "--all") {
			return ask(checkoutMsg)
		}
	case "reset":
		if contains(args, "--hard") {
			return ask(resetHardMsg)
		}
	case "stash":
		if len(args) == 0 {
			return ask(stashMsg)
		}
		switch args[0] {
		case "list", "show", "pop", "apply", "drop", "branch", "clear":
			return nil
		case "push", "save":
			return ask(stashMsg)
		}
		if strings.HasPrefix(args[0], "-") {
			return ask(stashMsg)
		}
	case "clean":
		if contains(args, "--force") || hasShortFlag(args, 'f') || hasShortFlag(args, 'd') || hasShortFlag(args, 'x') {
			return ask(cleanMsg)
		}
	}
	return nil
}
