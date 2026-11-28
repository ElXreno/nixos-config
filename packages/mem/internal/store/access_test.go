package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	testAgent = "deepseek"
	ownKey    = "zebra-protocol-notes"
	ownWord   = "zebra"
	sharedKey = "push-only-when-asked"
)

type views struct {
	primary, agent, borrowed, all *Store
	clock                         *clock
}

func openViews(t *testing.T) views {
	t.Helper()
	c := &clock{t: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	path := filepath.Join(t.TempDir(), "mem.db")
	open := func(a Access) *Store {
		s, err := Open(context.Background(), path, Options{Now: c.now, Access: a})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	}
	return views{
		primary:  open(Access{}),
		agent:    open(Access{Agent: testAgent}),
		borrowed: open(Access{See: []string{testAgent}}),
		all:      open(Access{All: true}),
		clock:    c,
	}
}

func seedViews(t *testing.T, v views) {
	t.Helper()
	ctx := context.Background()
	shared := spec{key: sharedKey, title: "Push only when asked", core: true,
		body: "Never push to the remote by default: wait until the owner asks for the push."}.req()
	shared.Triggers = &[]Trigger{{Tool: "Bash", Regex: `git push`}}
	mustPut(t, v.primary, shared)
	own := spec{key: ownKey, title: "Zebra protocol notes", core: true, scopes: []string{GlobalScope, "topic:lab"},
		body: "The zebra protocol answers on port 7001 and never on the remote; the owner said nothing leaves this machine."}.req()
	own.Triggers = &[]Trigger{{Tool: "Bash", Regex: ownWord}}
	own.Links = []Link{{Dst: sharedKey, Rel: Relates, Note: "zebra note about the push rule"}}
	mustPut(t, v.agent, own)
	mustPut(t, v.agent, spec{key: "zebra-standing", title: "Zebra standing rule", scopes: []string{GlobalScope, TopicScope(StandingTopic)},
		body: "Keep the zebra stand powered while a capture runs."}.req())
	state := spec{key: "zebra-stand-state", title: "Zebra stand state", kind: State, scopes: []string{GlobalScope},
		body: "The zebra stand runs revision four today."}.req()
	state.TTLDays = ptr(3)
	mustPut(t, v.agent, state)
	for _, key := range []string{sharedKey, ownKey} {
		rec, err := v.all.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := v.all.PutVectors(ctx, "m", "", []VecRow{{Key: key, Vec: []float32{1, 0}, TextSHA: TextSHA(DocText("", rec))}}); err != nil {
			t.Fatal(err)
		}
	}
}

func leaks(t *testing.T, name string, recs []Record) {
	t.Helper()
	for _, r := range recs {
		if r.Agent() != "" || strings.Contains(strings.ToLower(r.Key+r.Title+r.Body), ownWord) {
			t.Errorf("%s shows the agent record %s", name, r.Key)
		}
	}
}

func hitRecords(hits []Hit) []Record {
	out := make([]Record, len(hits))
	for i, h := range hits {
		out[i] = h.Record
	}
	return out
}

func TestPrimaryNeverSeesAgentRecords(t *testing.T) {
	ctx := context.Background()
	v := openViews(t)
	seedViews(t, v)
	s := v.primary

	if _, err := s.Get(ctx, ownKey); !errors.Is(err, ErrNotFound) {
		t.Errorf("get = %v, want not found", err)
	}
	list, err := s.List(ctx, Filter{AllStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("list has %d records, want the shared one", len(list))
	}
	leaks(t, "list", list)
	for _, mode := range []Mode{Fused, WordsOnly, TrigramOnly} {
		hits, err := s.Search(ctx, Query{Text: "zebra protocol port remote", Mode: mode, AllStatus: true, AllChunks: true})
		if err != nil {
			t.Fatal(err)
		}
		leaks(t, "search "+mode.String(), hitRecords(hits))
		words, tri, err := s.Lists(ctx, Query{Text: "zebra protocol port remote", Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range append(words, tri...) {
			if strings.Contains(h.Key, ownWord) {
				t.Errorf("lists %s shows %s", mode, h.Key)
			}
		}
	}
	core, err := s.Core(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	leaks(t, "core", core)
	coreAt, err := s.CoreAt(ctx, Here{})
	if err != nil {
		t.Fatal(err)
	}
	leaks(t, "core at", coreAt)
	standing, err := s.StandingAt(ctx, Here{})
	if err != nil {
		t.Fatal(err)
	}
	leaks(t, "standing", standing)
	out := filepath.Join(t.TempDir(), "core.md")
	if _, err := s.WriteCore(ctx, out); err != nil {
		t.Fatal(err)
	}
	if text, _ := os.ReadFile(out); strings.Contains(strings.ToLower(string(text)), ownWord) || !strings.Contains(string(text), sharedKey) {
		t.Errorf("core file = %q", text)
	}
	byTrigger, err := s.ListByTrigger(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	leaks(t, "list by trigger", byTrigger)
	for name, match := range map[string]func() ([]TriggerHit, error){
		"match triggers":    func() ([]TriggerHit, error) { return s.MatchTriggers(ctx, "Bash", "zebra capture", nil) },
		"match triggers at": func() ([]TriggerHit, error) { return s.MatchTriggersAt(ctx, "Bash", "zebra capture", Here{}) },
	} {
		hits, err := match()
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Errorf("%s fired %d hits", name, len(hits))
		}
	}
	topics, err := s.Topics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tp := range topics {
		if tp.Records != 0 {
			t.Errorf("topic %s counts %d records of the agent", tp.ID, tp.Records)
		}
	}
	topicMap, err := s.TopicMap(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(topicMap) != 0 {
		t.Errorf("topic map = %+v", topicMap)
	}
	if err := s.ScanVectors(ctx, "m", Query{AllStatus: true}, func(vs VecScan) error {
		if vs.Key != sharedKey {
			t.Errorf("vector scan shows %s", vs.Key)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if h, err := s.History(ctx, ownKey); err != nil || len(h) != 0 {
		t.Errorf("history = %+v, %v", h, err)
	}
	if links, err := s.Links(ctx, sharedKey); err != nil || len(links) != 0 {
		t.Errorf("links of the shared record = %+v, %v", links, err)
	}
	probe := Record{Key: "probe", Kind: Rule, Scopes: []string{GlobalScope}, Title: "Zebra protocol notes",
		Body: "The zebra protocol answers on port 7001 and never on the remote."}
	neigh, err := s.Neighbours(ctx, probe, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range neigh {
		leaks(t, "neighbours", []Record{c.Record})
	}
}

func TestPrimaryWritesStayApart(t *testing.T) {
	ctx := context.Background()
	v := openViews(t)
	seedViews(t, v)
	s := v.primary
	var fe *ForeignError

	twin := spec{key: "zebra-twin", title: "Zebra protocol notes",
		body: "The zebra protocol answers on port 7001 and never on the remote; the owner said nothing leaves this machine."}.req()
	if _, err := s.Put(ctx, twin); err != nil {
		t.Errorf("the duplicate gate named a hidden record: %v", err)
	}
	if _, err := s.Put(ctx, spec{key: ownKey, title: "Mine", body: "A record of the primary agent."}.req()); !errors.As(err, &fe) || fe.Op != "create" ||
		strings.Contains(err.Error(), testAgent) {
		t.Errorf("put on a taken key = %v", err)
	}
	if _, err := s.Put(ctx, PutRequest{Key: ownKey, Update: true, Body: ptr("changed"), Gate: DefaultGate()}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update = %v", err)
	}
	if _, err := s.Edit(ctx, EditRequest{Key: ownKey, Old: "zebra", New: "horse"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("edit = %v", err)
	}
	if _, err := s.Retire(ctx, ownKey, "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("retire = %v", err)
	}
	if _, err := s.Verify(ctx, ownKey, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("verify = %v", err)
	}
	tagged := spec{key: "tagged", title: "Tagged", body: "A record that names an agent scope by hand.", scopes: []string{GlobalScope, AgentScope(testAgent)}}.req()
	if _, err := s.Put(ctx, tagged); !errors.Is(err, ErrInvalid) {
		t.Errorf("put with an agent scope = %v", err)
	}
	if _, err := s.Share(ctx, ownKey, "asked"); !errors.Is(err, ErrNotFound) {
		t.Errorf("share without --as = %v", err)
	}
	if res, err := s.SetTopics(ctx, []TopicAssign{{Key: ownKey, Topic: "lab", Title: "Lab"}}); err != nil || !slices.Contains(res.Unknown, ownKey) {
		t.Errorf("set topics = %+v, %v", res, err)
	}

	v.clock.advance(5 * 24 * time.Hour)
	expired, err := s.Expire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].Key != "zebra-stand-state" || s.Access().Sees(expired[0].Scopes) {
		t.Errorf("expire = %+v", expired)
	}
	if r, err := v.agent.Get(ctx, "zebra-stand-state"); err != nil || r.Status != Expired {
		t.Errorf("the agent state record = %+v, %v", r.Status, err)
	}
}

func TestAgentReadsAllAndChangesItsOwn(t *testing.T) {
	ctx := context.Background()
	v := openViews(t)
	seedViews(t, v)
	s := v.agent
	var fe *ForeignError

	list, err := s.List(ctx, Filter{})
	if err != nil || len(list) != 4 {
		t.Fatalf("the agent lists %d records, %v", len(list), err)
	}
	own, err := s.Get(ctx, ownKey)
	if err != nil || own.Agent() != testAgent {
		t.Fatalf("own record = %+v, %v", own.Scopes, err)
	}
	if hits, err := s.Search(ctx, Query{Text: "push remote owner"}); err != nil || !slices.Contains(keys(hits), sharedKey) {
		t.Errorf("the agent search misses the shared record: %v, %v", keys(hits), err)
	}
	if links, err := s.Links(ctx, sharedKey); err != nil || len(links) != 1 {
		t.Errorf("links = %+v, %v", links, err)
	}
	for name, change := range map[string]func() error{
		"update": func() error {
			_, err := s.Put(ctx, PutRequest{Key: sharedKey, Update: true, Body: ptr("changed by the agent"), Gate: DefaultGate()})
			return err
		},
		"edit": func() error {
			_, err := s.Edit(ctx, EditRequest{Key: sharedKey, Old: "remote", New: "zebra"})
			return err
		},
		"retire": func() error { _, err := s.Retire(ctx, sharedKey, "the agent disagrees"); return err },
		"verify": func() error { _, err := s.Verify(ctx, sharedKey, ""); return err },
		"supersede": func() error {
			req := spec{key: "zebra-push-rule", title: "Zebra push rule", body: "Push whenever the zebra stand is idle."}.req()
			req.Supersedes = []string{sharedKey}
			_, err := s.Put(ctx, req)
			return err
		},
		"set topics": func() error {
			_, err := s.SetTopics(ctx, []TopicAssign{{Key: sharedKey, Topic: "lab", Title: "Lab"}})
			return err
		},
	} {
		if err := change(); !errors.As(err, &fe) || fe.Owner != "" {
			t.Errorf("%s of a shared record by the agent = %v", name, err)
		}
	}
	if r, err := v.primary.Get(ctx, sharedKey); err != nil || r.Version != 1 || strings.Contains(r.Body, ownWord) {
		t.Errorf("the shared record changed: %+v, %v", r, err)
	}
	if _, err := s.Put(ctx, PutRequest{Key: ownKey, Update: true, Scopes: []string{GlobalScope}, Body: ptr("The zebra protocol moved to port 7002."), Gate: DefaultGate()}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.Get(ctx, ownKey); r.Agent() != testAgent || r.Version != 2 {
		t.Errorf("own record after an update with new scopes = %+v", r)
	}
	other := spec{key: "other-agent", title: "Other", body: "A record with the scope of another agent.", scopes: []string{GlobalScope, AgentScope("qwen")}}.req()
	if _, err := s.Put(ctx, other); !errors.Is(err, ErrInvalid) {
		t.Errorf("put with a foreign agent scope = %v", err)
	}
	if _, err := s.Share(ctx, ownKey, "wanted"); !errors.Is(err, ErrInvalid) {
		t.Errorf("share by the agent = %v", err)
	}
	if _, err := v.primary.Get(ctx, ownKey); !errors.Is(err, ErrNotFound) {
		t.Errorf("the primary agent sees the record after the agent updated it: %v", err)
	}
}

func TestBorrowedViewReadsAndShares(t *testing.T) {
	ctx := context.Background()
	v := openViews(t)
	seedViews(t, v)
	s := v.borrowed

	if r, err := s.Get(ctx, ownKey); err != nil || r.Agent() != testAgent {
		t.Fatalf("borrowed get = %+v, %v", r, err)
	}
	if hits, err := s.Search(ctx, Query{Text: "zebra protocol port"}); err != nil || !slices.Contains(keys(hits), ownKey) {
		t.Errorf("borrowed search = %v, %v", keys(hits), err)
	}
	if _, err := s.Put(ctx, PutRequest{Key: ownKey, Update: true, Body: ptr("changed"), Gate: DefaultGate()}); !errors.Is(err, ErrInvalid) {
		t.Errorf("update through the borrowed view = %v", err)
	}
	if _, err := s.Put(ctx, spec{key: "fresh", title: "Fresh", body: "A new record written through the borrowed view."}.req()); !errors.Is(err, ErrInvalid) {
		t.Errorf("put through the borrowed view = %v", err)
	}
	if _, err := s.Retire(ctx, ownKey, "no"); !errors.Is(err, ErrInvalid) {
		t.Errorf("retire through the borrowed view = %v", err)
	}
	if _, err := s.Share(ctx, ownKey, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("share without a reason = %v", err)
	}
	shared, err := s.Share(ctx, ownKey, "the owner asked for it")
	if err != nil || shared.Agent() != "" || shared.Version != 2 {
		t.Fatalf("share = %+v, %v", shared, err)
	}
	if r, err := v.primary.Get(ctx, ownKey); err != nil || r.Agent() != "" {
		t.Errorf("the primary agent after the share = %+v, %v", r, err)
	}
	if h, err := v.primary.History(ctx, ownKey); err != nil || len(h) != 1 || !strings.Contains(h[0].Reason, "shared from agent "+testAgent) {
		t.Errorf("history after the share = %+v, %v", h, err)
	}
	if _, err := v.agent.Put(ctx, PutRequest{Key: ownKey, Update: true, Body: ptr("the agent wants it back"), Gate: DefaultGate()}); err == nil {
		t.Error("the agent changed a record that was shared")
	}
	wrong := Access{See: []string{"qwen"}}
	if wrong.Sees([]string{GlobalScope, AgentScope(testAgent)}) {
		t.Error("a view of another agent shows this one")
	}
}

func TestAgentScopeRules(t *testing.T) {
	if _, err := NormalizeScopes([]string{"global", "agent:deepseek", "agent:qwen"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("two agent scopes = %v", err)
	}
	r := Record{Key: "k", Kind: Fact, Scopes: []string{"agent:deepseek"}, Body: "body"}
	if err := r.normalize(); !errors.Is(err, ErrInvalid) {
		t.Errorf("a record with only an agent scope = %v", err)
	}
	if _, err := Open(context.Background(), filepath.Join(t.TempDir(), "mem.db"), Options{Access: Access{Agent: "bad name"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("open with a bad agent name = %v", err)
	}
	for where, want := range map[string]string{
		"WHERE r.key = ?":                     "WHERE V AND (r.key = ?)",
		"WHERE r.core = 1 ORDER BY r.key":     "WHERE V AND (r.core = 1) ORDER BY r.key",
		"WHERE a = ? OR b = ? ORDER BY r.key": "WHERE V AND (a = ? OR b = ?) ORDER BY r.key",
	} {
		if got := strings.Replace((Access{}).scoped(where), (Access{}).clause(), "V", 1); got != want {
			t.Errorf("scoped(%q) = %q", where, got)
		}
	}
	if got := (Access{Agent: "deepseek"}).scoped("WHERE r.key = ?"); got != "WHERE r.key = ?" {
		t.Errorf("an agent reads through %q", got)
	}
}
