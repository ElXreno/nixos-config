package recall

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mem/internal/store"
	"mem/internal/text"
)

func filler(n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("filler%04d", i)
	}
	return strings.Join(words, " ")
}

var stageA = Gate{MinCover: 0.3, MinShared: 2, MinTopic: 1}

var records = map[string]store.Record{
	"push": {Key: "no-push-default", Title: "No git push by default",
		Body: "Do not git push by default. After a commit, wait for explicit authorization; every push is its own action."},
	"arc": {Key: "zfs-arc-ram", Title: "ZFS ARC size after boot",
		Body: "The ZFS ARC grows to half of RAM when zfs_arc_max is unset and gives memory back slowly under pressure.\n" +
			"ARC в ZFS после загрузки занимает половину памяти, если zfs_arc_max не задан, и отдаёт её медленно."},
	"volume": {Key: "feedback_no_volume_changes_without_ask", Title: "Громкость без спроса не трогать",
		Body: "Громкость не трогать без спроса: wpctl, pactl и amixer запускаются только после явного разрешения, даже с возвратом итога."},
	"state": {Key: "agent-guard-push-state", Body: "agent-guard refactor: 7 commits up to c00c712d are not pushed."},
	"port": {Key: "feedback_no_model_start_when_ram_tight", Title: "Port 8097 starts the production model",
		Body: "A request to 127.0.0.1:8097 starts the production Ornith model on SMOK with about 73 GB pinned; the system hangs when RAM is tight."},
	"cooler": {Key: "reference_bs3pro_command_0xdf_bricks", Title: "Command 0xDF bricks the BS3 Pro",
		Body: "Command 0xDF bricks the Flydigi BS3 Pro cooler; flydigictl never sends it."},
	"imperm": {Key: "impermanence-state-dirs", Title: "Каталоги состояния в impermanence",
		Body: "Каждому новому сервису нужны каталоги состояния в конфиге impermanence; без них сервис молча падает после деплоя."},
	"kurwa": {Key: "kurwa-samsung-fw", Title: "Samsung firmware 1B2QNXH7 is never installed",
		Body: "Samsung 990 Pro NVMe firmware 1B2QNXH7 breaks the drive on KURWA; the drive stays on its current firmware update level."},
	"long": {Key: "reference_zfs_arc_size", Title: "ZFS ARC size cap",
		Body: "zfs_arc_max caps the ARC on BIMBA at 8 GB.\n" + filler(500)},
}

func TestGate(t *testing.T) {
	g := stageA
	cases := []struct {
		rec, prompt string
		want        bool
	}{
		{"push", "Can I git push the commit to the remote now, or do I wait for your authorization first?", true},
		{"arc", "Почему ARC в ZFS съедает всю память после загрузки и не отдаёт её?", true},
		{"volume", "убавь громкость через wpctl на динамиках", true},
		{"state", "is the agent-guard refactor pushed yet?", true},
		{"port", "curl 127.0.0.1:8097/v1/models to see which model is loaded", true},
		{"cooler", "The BS3 Pro cooler stopped responding after flydigictl sent a raw command, what now?", true},
		{"imperm", "После деплоя новый сервис на SMOK не стартует, в логах нет его каталога состояния", true},
		{"kurwa", "Should I install the Samsung 990 Pro firmware 1B2QNXH7 update on the NVMe drive?", true},
		{"long", "как ограничить ZFS ARC size", true},
		{"long", "zfs arc", true},
		{"long", "What is the ZFS ARC size cap on BIMBA?", true},
		{"arc", "Explain quantum chromodynamics lattice simulation parameters in detail, with references.", false},
		{"arc", "check the memory usage of the tablet", false},
		{"port", "the model answers slowly today", false},
		{"imperm", "почини сборку пакета в nixpkgs", false},
		{"push", "rename the variable in the Go test file", false},
		{"push", "write a commit message for this change", false},
		{"cooler", "send the report to the printer", false},
		{"volume", "сделай погромче", false},
		{"state", "refactor the parser into two files", false},
		{"kurwa", "update the flake inputs", false},
		{"push", "after the commit hook runs, wait a second and retry the action", false},
		{"cooler", "the raw command sent by the printer driver never returns", false},
		{"imperm", "в логах нового сервиса нет ошибок, он стартует нормально", false},
		{"long", "half of the boot logs are missing", false},
	}
	lowPass, highFail := math.Inf(1), 0.0
	for _, c := range cases {
		q := text.NewSet(c.prompt)
		rv := Measure(q, records[c.rec])
		got := g.Passes(rv)
		t.Logf("%-6s want %-5v cover %.3f shared %d topic %d", c.rec, c.want, rv.Cover, rv.Shared, rv.Topic)
		if got != c.want {
			t.Errorf("%s / %q: passes %v (cover %.3f, shared %d, topic %d), want %v", c.rec, c.prompt, got, rv.Cover, rv.Shared, rv.Topic, c.want)
		}
		switch {
		case c.want:
			lowPass = min(lowPass, rv.Cover)
		case rv.Shared >= min(g.MinShared, len(q)) && rv.Topic >= g.MinTopic:
			highFail = max(highFail, rv.Cover)
		}
	}
	if lowPass-g.MinCover < 0.05 {
		t.Errorf("weakest relevant pair %.3f sits within 0.05 of --min-cover %.2f", lowPass, g.MinCover)
	}
	t.Logf("weakest relevant %.3f, strongest irrelevant with enough shared and topic stems %.3f", lowPass, highFail)
}

func TestCoverIgnoresRecordLength(t *testing.T) {
	q := text.NewSet("как ограничить ZFS ARC size")
	short := store.Record{Key: "reference_zfs_arc_size", Title: "ZFS ARC size cap", Body: "zfs_arc_max caps the ARC on BIMBA at 8 GB."}
	long := records["long"]
	a, b := Measure(q, short), Measure(q, long)
	if a != b {
		t.Errorf("relevance changed with filler: short %+v long %+v", a, b)
	}
}

func TestMinSharedShortQuery(t *testing.T) {
	g := DefaultGate()
	q := text.NewSet("0xdf")
	rv := Measure(q, records["cooler"])
	if !g.Passes(rv) {
		t.Errorf("one-term query matching the record: %+v", rv)
	}
	if empty := text.NewSet("и в на"); g.Passes(Measure(empty, records["cooler"])) {
		t.Error("a query without informative terms passed")
	}
}

func hitsOf(recs ...store.Record) []store.Hit {
	out := make([]store.Hit, len(recs))
	for i, r := range recs {
		out[i] = store.Hit{Record: r}
	}
	return out
}

func TestChoose(t *testing.T) {
	push, cooler := records["push"], records["cooler"]
	saga := store.Record{Key: "project_saga#1", Kind: store.Log, Title: "git push saga", Body: "git push of the zfs readahead batch"}
	cases := []struct {
		name  string
		query string
		recs  []store.Record
		limit int
		want  []string
	}{
		{"passes on topic and shared stems", "may I git push now?", []store.Record{push, cooler}, 4, []string{"no-push-default"}},
		{"log records never pass", "git push saga", []store.Record{saga, push}, 4, []string{"no-push-default"}},
		{"a one-stem query needs one shared stem", "push", []store.Record{push}, 4, []string{"no-push-default"}},
		{"cover counts every query stem", "push cooler printer driver flake", []store.Record{push}, 4, nil},
		{"topic stem required", "wait for explicit authorization", []store.Record{push}, 4, nil},
		{"limit bounds the output", "git push BS3 Pro cooler command", []store.Record{push, cooler}, 1, []string{"no-push-default"}},
		{"no limit", "git push BS3 Pro cooler command", []store.Record{push, cooler}, 0, []string{"no-push-default", "reference_bs3pro_command_0xdf_bricks"}},
	}
	for _, c := range cases {
		cands := Candidates(c.query, hitsOf(c.recs...))
		var got []string
		for _, i := range stageA.Choose(cands, c.limit) {
			got = append(got, cands[i].Hit.Record.Key)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMinSharedZero(t *testing.T) {
	g := Gate{MinCover: 0.3, MinShared: 0, MinTopic: 1}
	q := text.NewSet("may I git push now?")
	if rv := Measure(q, records["push"]); !g.Passes(rv) {
		t.Errorf("--min-shared 0 rejected a passing record: %+v", rv)
	}
	if rv := Measure(text.NewSet("и в на"), records["push"]); g.Passes(rv) {
		t.Errorf("a query without informative terms passed: %+v", rv)
	}
	if !(Gate{}).Passes(Relevance{Terms: 3}) {
		t.Error("a gate without minimums rejected a query with terms")
	}
}

func TestTopRuleIgnoresPool(t *testing.T) {
	ctx := t.Context()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "m.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	recs := []store.Record{
		{Key: "top", Kind: store.Fact, Scopes: []string{"global"}, Title: "Fan cooler 0xDF", Body: "0xDF 0xDF on the fan cooler."},
		{Key: "runner", Kind: store.Fact, Scopes: []string{"global"}, Title: "Fan cooler notes", Body: "Fan cooler. " + filler(300) + " 0xDF"},
	}
	for i := range 65 {
		recs = append(recs, store.Record{Key: fmt.Sprintf("tri-%02d", i), Kind: store.Fact, Scopes: []string{"global"}, Body: fmt.Sprintf("zz0xdfzz q%02d", i)})
	}
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	q := "fan cooler 0xDF"
	g := Gate{MinCover: 2, MinShared: 1, Lead: true, LeadShared: 1}
	var picks [][]string
	for _, pool := range []int{DefaultPool, 20} {
		hits, err := s.Search(ctx, SearchQuery(q, pool, nil, nil))
		if err != nil {
			t.Fatal(err)
		}
		cands := Candidates(q, hits)
		var got []string
		for _, i := range g.Choose(cands, 4) {
			got = append(got, cands[i].Hit.Record.Key)
		}
		if len(cands) > 1 {
			t.Logf("pool %d: top %s words %d tri %d, runner-up %s words %d tri %d", pool, cands[0].Hit.Record.Key, cands[0].Hit.WordsRank, cands[0].Hit.TriRank,
				cands[1].Hit.Record.Key, cands[1].Hit.WordsRank, cands[1].Hit.TriRank)
		}
		picks = append(picks, got)
	}
	if !slices.Equal(picks[0], []string{"top"}) || !slices.Equal(picks[0], picks[1]) {
		t.Errorf("top-hit rule picked %v at pool %d and %v at pool 20", picks[0], DefaultPool, picks[1])
	}
}

func TestLead(t *testing.T) {
	cand := func(key string, words, tri int, cover float64, shared, topic int) Candidate {
		return Candidate{Hit: store.Hit{Record: store.Record{Key: key}, WordsRank: words, TriRank: tri},
			Relevance: Relevance{Cover: cover, Shared: shared, Topic: topic, Terms: 7}}
	}
	top := cand("top", 1, 1, 0.286, 2, 2)
	second := cand("second", 2, 0, 0.286, 2, 1)
	g := Gate{MinCover: 0.3, MinShared: 2, MinTopic: 1, LeadShared: 2}
	cases := []struct {
		name  string
		lead  bool
		cands []Candidate
		want  []string
	}{
		{"off", false, []Candidate{top, second}, nil},
		{"a top hit first in both tables skips the cover floor", true, []Candidate{top, second}, []string{"top"}},
		{"runner-up also matches in the trigram table", true, []Candidate{top, cand("second", 2, 2, 0.286, 2, 1)}, nil},
		{"top hit second in the words table", true, []Candidate{cand("top", 2, 1, 0.286, 2, 2), cand("second", 1, 0, 0.286, 2, 1)}, nil},
		{"top hit without a trigram match", true, []Candidate{cand("top", 1, 0, 0.286, 2, 2), second}, nil},
		{"a lone hit first in both tables", true, []Candidate{top}, []string{"top"}},
		{"shared floor still holds", true, []Candidate{cand("weak", 1, 1, 0.286, 1, 2), second}, nil},
		{"topic floor still holds", true, []Candidate{cand("off", 1, 1, 0.286, 2, 0), second}, nil},
		{"only the top hit", true, []Candidate{top, cand("third", 3, 0, 0.286, 2, 1)}, []string{"top"}},
		{"own shared floor", true, []Candidate{top, second}, []string{"top"}},
	}
	for _, c := range cases {
		g.Lead, g.MinShared = c.lead, 2
		if c.name == "own shared floor" {
			g.MinShared = 4
		}
		var got []string
		for _, i := range g.Choose(c.cands, 4) {
			got = append(got, c.cands[i].Hit.Record.Key)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHookSearchQueryIsStrict(t *testing.T) {
	if q := SearchQuery("и в на", 12, nil, nil); q.Fallback {
		t.Errorf("the prompt search must not fall back to raw terms: %+v", q)
	}
}
