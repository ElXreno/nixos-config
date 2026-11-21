package verdict

import "fmt"

type Level int

const (
	Silent Level = iota
	Advise
	Ask
	Deny
)

var names = [...]string{Silent: "silent", Advise: "advise", Ask: "ask", Deny: "deny"}

func (l Level) String() string {
	if l < Silent || l > Deny {
		return fmt.Sprintf("level(%d)", int(l))
	}
	return names[l]
}

func ParseLevel(s string) (Level, error) {
	switch s {
	case "silent", "0":
		return Silent, nil
	case "advise", "1":
		return Advise, nil
	case "ask":
		return Ask, nil
	case "deny", "2":
		return Deny, nil
	}
	return Silent, fmt.Errorf("unknown level %q", s)
}

func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

func (l *Level) UnmarshalText(b []byte) error {
	parsed, err := ParseLevel(string(b))
	if err != nil {
		return err
	}
	*l = parsed
	return nil
}

type Outcome string

const (
	Rewrite  Outcome = "rewrite"
	Dialog   Outcome = "dialog"
	Approved Outcome = "approved"
	Declined Outcome = "declined"
	Failed   Outcome = "error"
)

func (l Level) Outcome() Outcome { return Outcome(l.String()) }

type Verdict struct {
	Level    Level
	Check    string
	Message  string
	Findings []string
	Fix      string
}

type Verdicts []Verdict

func (v Verdicts) Worst() Level {
	worst := Silent
	for _, x := range v {
		if x.Level > worst {
			worst = x.Level
		}
	}
	return worst
}

func (v Verdicts) At(l Level) Verdicts {
	var out Verdicts
	for _, x := range v {
		if x.Level == l {
			out = append(out, x)
		}
	}
	return out
}
