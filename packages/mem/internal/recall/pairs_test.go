package recall

import (
	"fmt"
	"slices"
	"testing"

	"mem/internal/store"
)

func pairsFixture() ([]store.Record, map[string][]float32) {
	push := "Do not git push by default; wait for an explicit authorization after every commit."
	zfs := "The ZFS ARC grows to half of the RAM when zfs_arc_max is unset."
	rec := func(key string, kind store.Kind, scope, body string) store.Record {
		return store.Record{Key: key, Kind: kind, Scopes: []string{scope}, Title: "T", Body: body, Status: store.Current}
	}
	old := rec("push-old", store.Rule, "global", push)
	old.Status = store.Retired
	recs := []store.Record{
		rec("zfs-f", store.Fact, "project:x", zfs),
		rec("push-a", store.Rule, "global", push),
		rec("push-b", store.Rule, "project:z", push),
		rec("push-c", store.Fact, "global", push),
		rec("push-log", store.Log, "global", push),
		old,
		rec("push-novec", store.Rule, "global", push),
		rec("zfs-d", store.Fact, "project:x", zfs),
		rec("zfs-e", store.Fact, "project:y", zfs),
		rec("bt-g", store.Rule, "global", "Bluetooth headset pairing fails on the tablet."),
		rec("bt-h", store.Rule, "global", "Kernel config of the router firmware."),
	}
	vecs := map[string][]float32{
		"push-a":   {1, 0, 0},
		"push-b":   {1, 0, 0},
		"push-c":   {0.96, 0.28, 0},
		"push-log": {1, 0, 0},
		"push-old": {1, 0, 0},
		"zfs-d":    {0, 1, 0},
		"zfs-e":    {0, 1, 0},
		"zfs-f":    {0, 0.95, 0.3122499},
		"bt-g":     {0, 0, 1},
		"bt-h":     {0, 0, 2},
	}
	return recs, vecs
}

func TestPairs(t *testing.T) {
	recs, vecs := pairsFixture()
	cases := []struct {
		name               string
		opt                PairOptions
		total, gated, past int
		want               []string
	}{
		{"past only", PairOptions{MinCos: 0.9}, 7, 2, 5, []string{
			"bt-g bt-h rule/rule 1.000 false",
			"zfs-d zfs-e fact/fact 1.000 false",
			"push-a push-c rule/fact 0.960 false",
			"push-b push-c rule/fact 0.960 false",
			"zfs-e zfs-f fact/fact 0.950 false",
		}},
		{"all", PairOptions{MinCos: 0.9, All: true}, 7, 2, 5, []string{
			"bt-g bt-h rule/rule 1.000 false",
			"push-a push-b rule/rule 1.000 true",
			"zfs-d zfs-e fact/fact 1.000 false",
			"push-a push-c rule/fact 0.960 false",
			"push-b push-c rule/fact 0.960 false",
			"zfs-d zfs-f fact/fact 0.950 true",
			"zfs-e zfs-f fact/fact 0.950 false",
		}},
		{"limit keeps the totals", PairOptions{MinCos: 0.9, Limit: 2}, 7, 2, 5, []string{
			"bt-g bt-h rule/rule 1.000 false",
			"zfs-d zfs-e fact/fact 1.000 false",
		}},
		{"higher cosine", PairOptions{MinCos: 0.99, All: true}, 3, 1, 2, []string{
			"bt-g bt-h rule/rule 1.000 false",
			"push-a push-b rule/rule 1.000 true",
			"zfs-d zfs-e fact/fact 1.000 false",
		}},
		{"none", PairOptions{MinCos: 1.01, All: true}, 0, 0, 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Pairs(recs, vecs, c.opt)
			var got []string
			for _, p := range res.Pairs {
				got = append(got, fmt.Sprintf("%s %s %s/%s %.3f %v", p.A, p.B, p.KindA, p.KindB, p.Cos, p.Gate))
			}
			if res.Records != 8 || res.Total != c.total || res.Gated != c.gated || res.Past != c.past || res.MinCos != c.opt.MinCos {
				t.Errorf("records %d total %d gated %d past %d min %v", res.Records, res.Total, res.Gated, res.Past, res.MinCos)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("pairs:\n%q\nwant\n%q", got, c.want)
			}
			if res.Pairs == nil {
				t.Error("pairs is nil, want an empty list")
			}
		})
	}
}

func TestPairsGateInputs(t *testing.T) {
	recs, vecs := pairsFixture()
	res := Pairs(recs, vecs, PairOptions{MinCos: 0.9, All: true})
	byPair := map[string]Pair{}
	for _, p := range res.Pairs {
		byPair[p.A+" "+p.B] = p
	}
	cases := []struct {
		pair      string
		minSim    float64
		maxSim    float64
		minShared int
		gate      bool
	}{
		{"push-a push-b", 0.45, 1, 2, true},
		{"push-a push-c", 0.45, 1, 2, false},
		{"zfs-d zfs-f", 0.45, 1, 2, true},
		{"zfs-d zfs-e", 0.45, 1, 2, false},
		{"bt-g bt-h", 0, 0.44, 0, false},
	}
	for _, c := range cases {
		p, ok := byPair[c.pair]
		if !ok {
			t.Errorf("%s: missing", c.pair)
			continue
		}
		if p.Sim < c.minSim || p.Sim > c.maxSim || p.Shared < c.minShared || p.Gate != c.gate {
			t.Errorf("%s: sim %v shared %d gate %v", c.pair, p.Sim, p.Shared, p.Gate)
		}
	}
}
