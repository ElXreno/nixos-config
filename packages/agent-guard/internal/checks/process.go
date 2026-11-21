package checks

import (
	"regexp"
	"strconv"

	"agent-guard/internal/harness"
	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

func backgroundMsg(v harness.Vocabulary) string {
	return "Blocked: do not background with '&'. " + v.Background + " This applies inside ssh, bash -c and container exec payloads too. If the ampersand is part of a URL or literal text, quote the argument."
}

func detachedMsg(v harness.Vocabulary) string {
	return "Blocked: do not detach with nohup/setsid/disown. " + v.Detach + " A detached shell job is invisible to all of that, so the only way to observe it is a sleep-and-poll loop."
}

func background(cmd shell.Command, cfg *Settings) *verdict.Verdict {
	if cmd.Detached {
		return deny(detachedMsg(cfg.spec().Vocab))
	}
	if cmd.Background {
		return deny(backgroundMsg(cfg.spec().Vocab))
	}
	return nil
}

func pkillMsg(v harness.Vocabulary) string {
	return "Blocked: pkill and killall match by pattern, and pkill -f matches against full command lines, including this agent's own shell: that is how it kills the session it is running in. " + v.StopTracked + " Otherwise list first and kill explicitly: pgrep -a <pattern> to see every match with its command line, check that none of them is something you did not mean, then kill <pid> for the ones you chose."
}

func pkill(cmd shell.Command, cfg *Settings) *verdict.Verdict {
	switch cmd.Head() {
	case "pkill", "killall":
		return deny(pkillMsg(cfg.spec().Vocab))
	}
	return nil
}

var sleepArg = regexp.MustCompile(`^([0-9]+)(?:\.[0-9]+)?([smhd]?)$`)

func sleepCheck(cmd shell.Command, _ *Settings) *verdict.Verdict {
	if cmd.Head() != "sleep" {
		return nil
	}
	worst := 0
	for _, a := range cmd.Args() {
		m := sleepArg.FindStringSubmatch(a)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "m":
			n *= 60
		case "h":
			n *= 3600
		case "d":
			n *= 86400
		}
		if n > worst {
			worst = n
		}
	}
	switch {
	case worst > 60:
		return deny("Blocked: sleep " + strconv.Itoa(worst) + "s. Never block for over a minute. Wait on the real condition instead: a polling loop at 20-30s, a --wait/--for flag, journalctl -f, or systemctl is-active in a loop. If the step truly takes minutes, poll it and report progress between checks rather than sleeping through it.")
	case worst > 30:
		return advise("sleep " + strconv.Itoa(worst) + "s exceeds the 30s preference: poll on the actual condition if you can, and keep loop iterations short.")
	}
	return nil
}
