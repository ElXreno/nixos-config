package replay

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"agent-guard/internal/call"
	"agent-guard/internal/guard"
	"agent-guard/internal/transcript"
	"agent-guard/internal/verdict"
)

// EditStats counts replayed edits, edits with a verdict, and verdicts per check.
type EditStats struct {
	transcript.Stats
	Edits, Hits int
	Verdicts    map[string]int
}

// Edits replays the file checks over the transcripts through the engine, one
// TSV row per verdict.
func Edits(src transcript.Source, root string, topOnly bool, e *guard.Engine, w io.Writer) (EditStats, error) {
	records, ts, err := transcript.Walk(src, root, topOnly)
	if err != nil {
		return EditStats{Stats: ts}, err
	}
	seen := map[string]struct{}{}
	var edits []transcript.Record
	for _, r := range records {
		if (r.Call.Kind != call.Edit && r.Call.Kind != call.Write) || r.Call.Change.Path == "" {
			continue
		}
		if _, dup := seen[r.Key]; dup {
			continue
		}
		seen[r.Key] = struct{}{}
		edits = append(edits, r)
	}
	results := make([]verdict.Verdicts, len(edits))
	transcript.Each(len(edits), func(i int) {
		results[i] = e.PreTool(edits[i].Call).Verdicts
	})

	st := EditStats{Stats: ts, Edits: len(edits), Verdicts: map[string]int{}}
	bw := bufio.NewWriter(w)
	for i, vs := range results {
		if len(vs) > 0 {
			st.Hits++
		}
		kind := "top"
		if edits[i].Sub {
			kind = "sub"
		}
		c := edits[i].Call
		for _, v := range vs {
			st.Verdicts[v.Level.String()+" "+v.Check]++
			msg := v.Message
			if len(v.Findings) > 0 {
				msg = strings.Join(v.Findings, "; ")
			}
			if _, err := fmt.Fprintf(bw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n", v.Level, c.Change.Path, short(msg), v.Check, c.Tool, kind, payload(c.Change)); err != nil {
				return st, err
			}
		}
	}
	return st, bw.Flush()
}

func payload(c call.Change) int {
	n := len(c.Content)
	for _, p := range c.Pairs {
		n += len(p.New)
	}
	return n
}
