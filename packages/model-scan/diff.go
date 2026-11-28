package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func lineDiff(a, b string, limit int) (int, []string) {
	x := strings.Split(strings.TrimRight(a, "\n"), "\n")
	y := strings.Split(strings.TrimRight(b, "\n"), "\n")
	n, m := len(x), len(y)
	if n*m > 16_000_000 {
		return n + m, []string{"templates too large for a line diff"}
	}
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	var out []string
	changed := 0
	for i, j := 0, 0; i < n || j < m; {
		switch {
		case i < n && j < m && x[i] == y[j]:
			i++
			j++
			continue
		case j < m && (i == n || dp[i][j+1] >= dp[i+1][j]):
			out = append(out, fmt.Sprintf("+%4d %s", j+1, y[j]))
			j++
		default:
			out = append(out, fmt.Sprintf("-%4d %s", i+1, x[i]))
			i++
		}
		changed++
	}
	if len(out) > limit {
		out = append(out[:limit], fmt.Sprintf("... %d more changed lines", len(out)-limit))
	}
	return changed, out
}

var repoRe = regexp.MustCompile(`^[\w.-]+/[\w.-]+(@[\w./-]+)?$`)

func diffBase(t *target, o opts) {
	if len(t.tmpls) == 0 {
		return
	}
	var refs []namedText
	switch o.base {
	case "":
		if len(t.bases) > 0 {
			t.add(info, t.name, "declared base model: %s (diff the templates with -base auto)", strings.Join(t.bases, ", "))
		}
		return
	case "auto":
		if len(t.bases) == 0 {
			t.add(info, t.name, "no base model declared: the chat templates are not diffed, pass -base owner/repo")
			return
		}
		for _, b := range t.bases {
			if !repoRe.MatchString(b) {
				t.add(info, t.name, "declared base %q is not a Hugging Face repository id", b)
				continue
			}
			r, err := o.hf.templates(b)
			if err != nil {
				t.add(errs, t.name, "base %s: %v", b, err)
				continue
			}
			refs = append(refs, r...)
		}
	default:
		if st, err := os.Stat(o.base); err == nil && !st.IsDir() {
			b, err := os.ReadFile(o.base)
			if err != nil {
				t.add(errs, t.name, "%v", err)
				return
			}
			refs = []namedText{{o.base, string(b)}}
			break
		}
		r, err := o.hf.templates(o.base)
		if err != nil {
			t.add(errs, t.name, "base %s: %v", o.base, err)
			return
		}
		refs = r
	}
	if len(refs) == 0 {
		t.add(errs, t.name, "the base has no chat template to diff against")
		return
	}
	for _, h := range t.templateOrder() {
		tm := t.tmpls[h]
		best, bestName := -1, ""
		var bestOut []string
		for _, r := range refs {
			c, out := lineDiff(r.text, tm.text, 80)
			if best < 0 || c < best {
				best, bestName, bestOut = c, r.name, out
			}
		}
		if best == 0 {
			t.add(info, tm.files[0], "chat template identical to %s", bestName)
		} else {
			t.add(info, tm.files[0], "chat template differs from %s in %d lines, read every change:\n%s", bestName, best, strings.Join(bestOut, "\n"))
		}
	}
}
