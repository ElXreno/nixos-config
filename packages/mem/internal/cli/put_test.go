package cli

import (
	"strings"
	"testing"
)

func TestPutSyntax(t *testing.T) {
	sb := newSandbox(t)
	example := "example: mem put KEY --kind fact --scope global --title T --body TEXT"
	steps := []struct {
		name   string
		stdin  string
		args   []string
		code   int
		stdout []string
		stderr []string
		absent []string
	}{
		{name: "positional key then flags", args: []string{"put", "alpha-note", "--kind", "fact", "--scope", "global", "--title", "Alpha", "--body", "alpha body text one"},
			stdout: []string{"alpha-note\tcreated\tversion 1"}},
		{name: "flags then positional key", args: []string{"put", "--kind", "fact", "--scope", "global", "--title", "Beta", "--body", "beta body words two", "beta-note"},
			stdout: []string{"beta-note\tcreated\tversion 1"}},
		{name: "flags around positional key", args: []string{"put", "--kind", "fact", "gamma-note", "--scope", "global", "--title", "Gamma", "--body", "gamma body phrase three"},
			stdout: []string{"gamma-note\tcreated\tversion 1"}},
		{name: "stdin without body flag", stdin: "delta stdin body four", args: []string{"put", "delta-note", "--kind", "fact", "--scope", "global", "--title", "Delta"},
			stdout: []string{"delta-note\tcreated\tversion 1"}},
		{name: "body dash reads stdin", stdin: "epsilon dash body five", args: []string{"put", "epsilon-note", "--kind", "fact", "--scope", "global", "--title", "Epsilon", "--body", "-"},
			stdout: []string{"epsilon-note\tcreated\tversion 1"}},
		{name: "body flag ignores stdin", stdin: "stdin text that must be ignored", args: []string{"put", "zeta-note", "--kind", "fact", "--scope", "global", "--title", "Zeta", "--body", "zeta flag body six"},
			stdout: []string{"zeta-note\tcreated\tversion 1"}},
		{name: "key flag and same positional", args: []string{"put", "--key", "eta-note", "eta-note", "--kind", "fact", "--scope", "global", "--title", "Eta", "--body", "quantum sheep migrate across frozen deserts"},
			stdout: []string{"eta-note\tcreated\tversion 1"}},
		{name: "key flag and other positional", args: []string{"put", "--key", "theta-note", "iota-note", "--kind", "fact", "--scope", "global", "--body", "x"},
			code: 2, stderr: []string{"mem put: key given twice", example}},
		{name: "two positionals", args: []string{"put", "kappa-note", "extra", "--kind", "fact", "--scope", "global", "--body", "x"},
			code: 2, stderr: []string{"mem put: unexpected arguments extra", example}},
		{name: "missing key", args: []string{"put", "--kind", "fact", "--body", "x"},
			code: 2, stderr: []string{"mem put: missing KEY", example}},
		{name: "missing kind", args: []string{"put", "lambda-note", "--scope", "global", "--body", "x"},
			code: 2, stderr: []string{"mem put: missing --kind (new record). " + example}},
		{name: "unknown flag is compact", args: []string{"put", "mu-note", "--update", "--bdy", "x"},
			code: 2, stderr: []string{"mem put: flag provided but not defined: -bdy. " + example}, absent: []string{"usage:", "-scope value"}},
		{name: "supersedes itself", args: []string{"put", "alpha-note", "--supersedes", "alpha-note", "--kind", "fact", "--scope", "global", "--body", "x"},
			code: 2, stderr: []string{"--supersedes alpha-note names the record itself; use --update to change it"}},
		{name: "update body only keeps metadata", args: []string{"put", "alpha-note", "--update", "--body", "alpha body text changed eight"},
			stdout: []string{"alpha-note\tupdated\tversion 2"}},
		{name: "get after update", args: []string{"get", "alpha-note"},
			stdout: []string{"alpha body text changed eight", "scopes: global", "title: Alpha"}},
		{name: "help prints full usage", args: []string{"put", "--help"},
			code: 0, stderr: []string{"usage: mem put KEY", "--update keeps kind", "-body string"}},
		{name: "dashdash as flag value is not a terminator", args: []string{"put", "--title", "--", "nu-note", "--kind", "fact", "--scope", "global", "--body", "quasar nebula pulsar"},
			stdout: []string{"nu-note\tcreated\tversion 1"}},
		{name: "dashdash value kept as title", args: []string{"get", "nu-note"},
			stdout: []string{"title: --", "quasar nebula pulsar"}},
		{name: "dashdash terminator after flags", args: []string{"put", "--kind", "fact", "--scope", "global", "--title", "Xi", "--body", "granite basalt obsidian", "--", "xi-note"},
			stdout: []string{"xi-note\tcreated\tversion 1"}},
		{name: "dashdash value then terminator", args: []string{"put", "--kind", "fact", "--scope", "global", "--body", "--", "--", "omicron-note", "--title", "Omicron"},
			code: 2, stderr: []string{"mem put: unexpected arguments --title Omicron"}},
		{name: "no-arg command example has no trailing space", args: []string{"expire", "--nope"},
			code: 2, stderr: []string{"example: mem expire\n"}},
		{name: "get unknown flag is compact", args: []string{"get", "alpha-note", "--nope"},
			code: 2, stderr: []string{"mem get: flag provided but not defined: -nope. example: mem get KEY... [--body] [--history]"}, absent: []string{"usage:"}},
	}
	for _, st := range steps {
		code, out, errs := sb.run(st.stdin, st.args...)
		if code != st.code {
			t.Errorf("%s: exit %d, want %d\nstdout: %s\nstderr: %s", st.name, code, st.code, out, errs)
			continue
		}
		for _, w := range st.stdout {
			if !strings.Contains(out, w) {
				t.Errorf("%s: stdout lacks %q:\n%s", st.name, w, out)
			}
		}
		for _, w := range st.stderr {
			if !strings.Contains(errs, w) {
				t.Errorf("%s: stderr lacks %q:\n%s", st.name, w, errs)
			}
		}
		for _, w := range st.absent {
			if strings.Contains(errs, w) || strings.Contains(out, w) {
				t.Errorf("%s: output has %q:\nstdout: %s\nstderr: %s", st.name, w, out, errs)
			}
		}
	}
}
