package hook

import (
	"context"
	"testing"
	"time"

	"mem/internal/store"
)

func TestVectorPathHonoursScopeStatusAndSessionDedup(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.seedWith(t, []store.Record{
		{Key: "elsewhere-push", Kind: store.Rule, Scopes: []string{"project:elsewhere"}, Title: "Push upstream", Body: "In the elsewhere project commits go upstream at once."},
		{Key: "retired-push", Kind: store.Rule, Scopes: []string{"global"}, Title: "Push upstream", Body: "Commits go upstream at once, no confirmation."},
	})
	sb.indexVectors(t)
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retire(ctx, "retired-push", "test"); err != nil {
		t.Fatal(err)
	}
	info, err := s.VectorInfo(ctx, fakeModel)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := info["retired-push"]; !ok {
		t.Fatal("the vector of the retired record is gone, the test would prove nothing")
	}
	_ = s.Close()
	useEmbedder(t, &keywordEmbedder{})
	args := append(append([]string{}, vecFlags...), "--reg-take", "10", "--reg-cos", "0.3")
	for _, pool := range []string{"reg", "fused-reg", "all", "fused-all"} {
		a := append(append([]string{}, args...), "--vec-pool", pool)
		sb.check(t, spec{name: pool, event: UserPromptSubmit, stdin: promptInput(t, sb, "f-"+pool, "Should I send my commits upstream already?"),
			args: a, has: []string{"`no-push-default` [rule"}, lacks: []string{"elsewhere-push", "retired-push"}, log: []string{"embed=ok"}})
	}
}

func TestRecordWithoutAVectorStaysRecallableThroughTheLexicalGate(t *testing.T) {
	const prompt = "May I tag a release before the nightly-bake job is green?"
	for _, pool := range []string{"reg", "fused-reg", "all", "fused-all"} {
		t.Run(pool, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			sb.indexVectors(t)
			sb.seedWith(t, []store.Record{{Key: "release-needs-bake", Kind: store.Rule, Scopes: []string{"global"}, Title: "Releases wait for the nightly-bake job",
				Body: "Never tag a release before the nightly-bake job is green; the bake job runs the full test matrix."}})
			useEmbedder(t, &keywordEmbedder{})
			sb.check(t, spec{name: "lexical only", event: UserPromptSubmit, stdin: promptInput(t, sb, "lex", prompt), has: []string{"release-needs-bake"}})
			args := append(append([]string{}, vecFlags...), "--vec-pool", pool)
			sb.check(t, spec{name: pool, event: UserPromptSubmit, stdin: promptInput(t, sb, "vec", prompt), args: args, has: []string{"release-needs-bake"}, log: []string{"embed=ok"}})
		})
	}
}
