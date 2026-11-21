package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestProjectScopeNames(t *testing.T) {
	for _, ok := range []string{
		"project:example-org/infra", "project:Example-Org/infra", "project:example-org/team/sub/parser",
		"project:~/tmp/royal-cludge-software", "project:~", "project:elxreno/.github", "project:nixos-config",
		"project:a.b/c_d+e@f-g", "project:srv/repo",
	} {
		if _, err := NormalizeScope(ok); err != nil {
			t.Errorf("NormalizeScope(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{
		"project:/abs", "project:a/", "project:a//b", "project:/", "project:", "project:a/./b", "project:a/../b",
		"project:..", "project:a/b c", "project:-a", "project:.a/b", "project:a\\b",
		"host:a/b", "host:~x", "topic:a/b", "topic:~", "host:smok/", "global/x",
	} {
		if got, err := NormalizeScope(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("NormalizeScope(%q) = %q, %v, want ErrInvalid", bad, got, err)
		}
	}
	if got, _ := NormalizeScope("Project:Example-Org/Infra"); got != "project:example-org/infra" {
		t.Errorf("lowercased %q", got)
	}
	if ProjectScope("Example-Org/infra") != "project:example-org/infra" {
		t.Error("ProjectScope keeps the slash")
	}
}

func TestPutAcceptsSlashedProjects(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	req := spec{key: "infra-note", body: "Infra repo note."}.req()
	req.Scopes = []string{"project:example-org/infra", "project:~/tmp/royal-cludge-software"}
	mustPut(t, s, req)
	got, err := s.Get(ctx, "infra-note")
	if err != nil || !slices.Equal(got.Scopes, []string{"project:example-org/infra", "project:~/tmp/royal-cludge-software"}) {
		t.Fatalf("%+v %v", got.Scopes, err)
	}
}

type placed struct {
	key    string
	scopes []string
}

var places = []placed{
	{"g", []string{GlobalScope}},
	{"p", []string{"project:p"}},
	{"hs", []string{"host:smok"}},
	{"hk", []string{"host:kurwa"}},
	{"p-hk", []string{"project:p", "host:kurwa"}},
	{"p-hs", []string{"project:p", "host:smok"}},
	{"g-hk", []string{GlobalScope, "host:kurwa"}},
	{"o-hs", []string{"project:other", "host:smok"}},
	{"o", []string{"project:other"}},
	{"hk-hs", []string{"host:kurwa", "host:smok"}},
	{"p-topic", []string{"project:p", "topic:t"}},
	{"o-topic", []string{"project:other", "topic:t"}},
	{"p-o-hk", []string{"project:p", "project:other", "host:kurwa"}},
}

func seedPlaces(t *testing.T) *Store {
	t.Helper()
	s, _ := newStore(t)
	trig, _ := ParseTrigger(`Bash:placemarker`)
	var recs []Record
	for _, p := range places {
		recs = append(recs, Record{Key: p.key, Kind: Rule, Scopes: p.scopes, Core: true, Title: "Place " + p.key, Body: "placemarker record " + p.key, Triggers: []Trigger{trig}})
	}
	if _, err := s.Upsert(context.Background(), recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHereDimensions(t *testing.T) {
	ctx := context.Background()
	s := seedPlaces(t)
	cases := []struct {
		name string
		here Here
		want []string
	}{
		{"smok in p", Here{Project: "project:p", Host: "host:smok", StrictHost: true}, []string{"g", "g-hk", "hk-hs", "hs", "o-hs", "p", "p-hs", "p-topic"}},
		{"smok in p, strict project", Here{Project: "project:p", Host: "host:smok", StrictHost: true, StrictProject: true}, []string{"g", "g-hk", "hk-hs", "hs", "p", "p-hs", "p-topic"}},
		{"kurwa in p", Here{Project: "project:p", Host: "host:kurwa", StrictHost: true}, []string{"g", "g-hk", "hk", "hk-hs", "p", "p-hk", "p-o-hk", "p-topic"}},
		{"smok outside a repo", Here{Host: "host:smok", StrictHost: true}, []string{"g", "g-hk", "hk-hs", "hs", "o-hs", "p-hs"}},
		{"smok outside a repo, strict", Here{Host: "host:smok", StrictHost: true, StrictProject: true}, []string{"g", "g-hk", "hk-hs", "hs"}},
		{"no host in p", Here{Project: "project:p", StrictHost: true}, []string{"g", "g-hk", "p", "p-topic"}},
		{"nowhere", Here{StrictHost: true}, []string{"g", "g-hk"}},
		{"smok in p, both off", Here{Project: "project:p", Host: "host:smok"}, []string{"g", "g-hk", "hk-hs", "hs", "o-hs", "p", "p-hk", "p-hs", "p-o-hk", "p-topic"}},
		{"smok in p, strict project only", Here{Project: "project:p", Host: "host:smok", StrictProject: true}, []string{"g", "g-hk", "hk-hs", "hs", "p", "p-hk", "p-hs", "p-o-hk", "p-topic"}},
		{"no host in p, both off", Here{Project: "project:p"}, []string{"g", "g-hk", "p", "p-hk", "p-hs", "p-o-hk", "p-topic"}},
	}
	for _, c := range cases {
		var direct []string
		for _, p := range places {
			if c.here.Matches(p.scopes) {
				direct = append(direct, p.key)
			}
		}
		slices.Sort(direct)
		if !slices.Equal(direct, c.want) {
			t.Errorf("%s: Matches %v, want %v", c.name, direct, c.want)
		}
		h := c.here
		hits, err := s.Search(ctx, Query{Text: "placemarker", Limit: 100, Here: &h})
		if err != nil {
			t.Fatal(err)
		}
		var found []string
		for _, x := range hits {
			found = append(found, x.Record.Key)
		}
		slices.Sort(found)
		if !slices.Equal(found, c.want) {
			t.Errorf("%s: Search %v, want %v", c.name, found, c.want)
		}
		core, err := s.CoreAt(ctx, c.here)
		if err != nil {
			t.Fatal(err)
		}
		var cored []string
		for _, r := range core {
			cored = append(cored, r.Key)
		}
		if !slices.Equal(cored, c.want) {
			t.Errorf("%s: CoreAt %v, want %v", c.name, cored, c.want)
		}
		trig, err := s.MatchTriggersAt(ctx, "Bash", "echo placemarker", c.here)
		if err != nil {
			t.Fatal(err)
		}
		var fired []string
		for _, x := range trig {
			fired = append(fired, x.Record.Key)
		}
		if !slices.Equal(fired, c.want) {
			t.Errorf("%s: MatchTriggersAt %v, want %v", c.name, fired, c.want)
		}
	}
}

func TestHereDefaultIsThePlainOr(t *testing.T) {
	ctx := context.Background()
	s := seedPlaces(t)
	for _, h := range []Here{{Project: "project:p", Host: "host:smok"}, {Project: "project:p"}, {Host: "host:kurwa"}, {}} {
		hh := h
		got, err := s.Search(ctx, Query{Text: "placemarker", Limit: 100, Here: &hh})
		if err != nil {
			t.Fatal(err)
		}
		want, err := s.Search(ctx, Query{Text: "placemarker", Limit: 100, Scopes: h.Scopes()})
		if err != nil {
			t.Fatal(err)
		}
		var a, b []string
		for _, x := range got {
			a = append(a, x.Record.Key)
		}
		for _, x := range want {
			b = append(b, x.Record.Key)
		}
		if !slices.Equal(a, b) {
			t.Errorf("%+v: %v, the scope OR gives %v", h, a, b)
		}
	}
}

func TestHereKeepsScopesOrForExplicitFilters(t *testing.T) {
	ctx := context.Background()
	s := seedPlaces(t)
	hits, err := s.Search(ctx, Query{Text: "placemarker", Limit: 100, Scopes: []string{"project:p"}})
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, x := range hits {
		found = append(found, x.Record.Key)
	}
	slices.Sort(found)
	if want := []string{"p", "p-hk", "p-hs", "p-o-hk", "p-topic"}; !slices.Equal(found, want) {
		t.Errorf("an explicit --scope filter stays a plain OR: %v", found)
	}
}
