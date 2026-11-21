package store

import "testing"

func TestListsMatchSearchRanks(t *testing.T) {
	s, _ := newStore(t)
	mustPut(t, s, spec{key: "cooler-firmware", title: "Cooler firmware", body: "The 0xDF command bricks the cooler firmware, never flash it"}.req())
	mustPut(t, s, spec{key: "push-rule", title: "No push", body: "Wait for explicit authorization before every git push command"}.req())
	mustPut(t, s, spec{key: "readahead", title: "ZFS readahead", body: "Batch readahead patches for ZFS and the firmware of the stand"}.req())
	mustPut(t, s, spec{key: "saga-log", title: "Saga", body: "ZFS readahead saga with many notes about the cooler", kind: Log}.req())
	for _, text := range []string{"cooler firmware 0xDF command", "git push authorization", "zfs readahead", "nothing matches here qqq"} {
		q := Query{Text: text, Limit: 50, Depth: 60, ExcludeKinds: []Kind{Log}, Mode: Fused}
		words, tri, err := s.Lists(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		hits, err := s.Search(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		wantW, wantT := map[string]int{}, map[string]int{}
		for _, h := range hits {
			if h.WordsRank > 0 {
				wantW[h.Record.Key] = h.WordsRank
			}
			if h.TriRank > 0 {
				wantT[h.Record.Key] = h.TriRank
			}
		}
		if len(words) != len(wantW) || len(tri) != len(wantT) {
			t.Fatalf("%q: lists %d/%d, search ranks %d/%d", text, len(words), len(tri), len(wantW), len(wantT))
		}
		for i, w := range words {
			if w.Rank != i+1 || wantW[w.Key] != w.Rank || w.Score <= 0 {
				t.Errorf("%q words %+v want rank %d", text, w, wantW[w.Key])
			}
		}
		for i, w := range tri {
			if w.Rank != i+1 || wantT[w.Key] != w.Rank {
				t.Errorf("%q trigram %+v want rank %d", text, w, wantT[w.Key])
			}
		}
		if w, tr, _ := s.Lists(t.Context(), Query{Text: text, Mode: WordsOnly, Depth: 60, ExcludeKinds: []Kind{Log}}); len(tr) != 0 || len(w) != len(wantW) {
			t.Errorf("%q words-only mode returned trigram %d", text, len(tr))
		}
	}
}
