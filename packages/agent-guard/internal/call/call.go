package call

import (
	"fmt"

	"agent-guard/internal/harness"
	"agent-guard/internal/text"
)

type Kind int

const (
	Other Kind = iota
	Shell
	Edit
	Write
)

var kindNames = [...]string{Other: "other", Shell: "shell", Edit: "edit", Write: "write"}

func (k Kind) String() string {
	if k < Other || k > Write {
		return "other"
	}
	return kindNames[k]
}

func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

func (k *Kind) UnmarshalText(b []byte) error {
	for i, n := range kindNames {
		if n == string(b) {
			*k = Kind(i)
			return nil
		}
	}
	return fmt.Errorf("unknown call kind %q", b)
}

type Pair struct {
	Old string
	New string
	All bool
}

type Apply int

const (
	Sequential Apply = iota
	Original
)

type Change struct {
	Path    string
	Write   bool
	Content string
	Pairs   []Pair
	Apply   Apply
}

type Call struct {
	Harness harness.ID
	Event   string
	Tool    string
	Kind    Kind
	Session string
	Agent   string
	CallID  string
	Cwd     string
	Command string
	Change  Change
	URL     string
}

func (c Call) StateKey() string {
	if c.Session == "" {
		return ""
	}
	k := string(c.Harness) + "-" + c.Session
	if c.Agent != "" {
		k += "-" + c.Agent
	}
	return k
}

func (c Call) Summary() string {
	for _, s := range []string{c.Command, c.Change.Path, c.URL} {
		if s != "" {
			return text.Clip(s, 300)
		}
	}
	return ""
}
