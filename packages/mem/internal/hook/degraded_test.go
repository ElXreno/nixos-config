package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
)

const noticeLead = "Agent memory: recall for this prompt was lexical only"

func withoutNotice(t *testing.T, out string) string {
	t.Helper()
	if out == "" {
		return ""
	}
	var env envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not hook JSON: %v\n%s", err, out)
	}
	ctx := env.HookSpecificOutput.AdditionalContext
	i := strings.LastIndex(ctx, noticeLead)
	if i < 0 {
		return out
	}
	ctx = strings.TrimRight(ctx[:i], "\n")
	if ctx == "" {
		return ""
	}
	data, err := encode(Output{HookEventName: env.HookSpecificOutput.HookEventName, AdditionalContext: ctx})
	if err != nil {
		t.Fatal(err)
	}
	return data + "\n"
}

func refused() error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
}

func TestDegradedNoticeOncePerOutage(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	down := &keywordEmbedder{err: refused()}
	up := &keywordEmbedder{}
	notice := fmt.Sprintf(degradedText, "down")
	if !strings.Contains(notice, "lexical only (embedder down)") || !strings.Contains(notice, "`mem search WORDS`") {
		t.Fatalf("notice text %q", notice)
	}
	ask := func(session, prompt string) string {
		return sb.context(t, promptInput(t, sb, session, prompt), append([]string{UserPromptSubmit}, vecFlags...)...)
	}
	const unrelated = "Explain quantum chromodynamics lattice simulation parameters in detail, with references."

	useEmbedder(t, down)
	if got := ask("d1", unrelated); got != notice {
		t.Fatalf("first failed prompt: %q, want only the notice", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "session=d1 none") || !strings.Contains(line, "embed=down") || !strings.Contains(line, "notice embed=down") {
		t.Errorf("log line %q", line)
	}
	turns := sb.turns(t)
	if last := turns[len(turns)-1]; last.Notice != noticeDegraded || last.Status != "none" || last.Embed != "down" {
		t.Errorf("turn %+v", last)
	}
	if got := ask("d1", unrelated+" Again."); got != "" {
		t.Errorf("second failed prompt of the same outage: %q", got)
	}
	got := ask("d1", "Can I git push the commit to the remote now, or do I wait for your authorization first?")
	if !strings.Contains(got, "`no-push-default` [rule") || strings.Contains(got, noticeLead) {
		t.Errorf("a lexical hit during a known outage carries no second notice:\n%s", got)
	}

	if got := ask("d2", "Can I git push the commit to the remote now, or do I wait for your authorization first?"); !strings.Contains(got, "`no-push-default` [rule") || !strings.HasSuffix(got, "\n"+notice) {
		t.Errorf("another session gets the records and its own notice:\n%s", got)
	}

	useEmbedder(t, up)
	if got := ask("d1", "Should I send my commits upstream already?"); strings.Contains(got, noticeLead) {
		t.Errorf("a working embedder still prints the notice:\n%s", got)
	}
	if got := ask("d1", unrelated+" Once more."); strings.Contains(got, noticeLead) {
		t.Errorf("a working embedder still prints the notice:\n%s", got)
	}

	useEmbedder(t, down)
	if got := ask("d1", unrelated+" Third."); got != notice {
		t.Errorf("a new outage after a recovery: %q, want the notice", got)
	}
	if got := ask("d1", unrelated+" Fourth."); got != "" {
		t.Errorf("the second outage repeats its notice: %q", got)
	}

	useEmbedder(t, &keywordEmbedder{err: errors.New("garbage reply")})
	fresh := newSandbox(t)
	fresh.seed(t)
	fresh.indexVectors(t)
	if got := fresh.context(t, promptInput(t, fresh, "d3", unrelated), append([]string{UserPromptSubmit}, vecFlags...)...); got != fmt.Sprintf(degradedText, "response") {
		t.Errorf("the notice names the outcome: %q", got)
	}
	if got := fresh.context(t, promptInput(t, fresh, "d4", unrelated), UserPromptSubmit); got != "" {
		t.Errorf("the lexical pool never prints the notice: %q", got)
	}
}

func TestDegradedNoticeSurvivesWithoutASession(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{err: refused()})
	stdin := `{"cwd":"` + sb.root + `","hook_event_name":"UserPromptSubmit","prompt":"Explain quantum chromodynamics lattice simulation parameters in detail."}`
	for range 2 {
		if got := sb.context(t, stdin, append([]string{UserPromptSubmit}, vecFlags...)...); !strings.HasPrefix(got, noticeLead) {
			t.Errorf("without a session id every failed prompt gets the notice: %q", got)
		}
	}
}
