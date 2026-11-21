package store

import (
	"context"
	"testing"
)

func TestEveryTextWritePathDropsTheVectorAndOthersKeepIt(t *testing.T) {
	ctx := context.Background()
	title := "A new title"
	cases := []struct {
		name string
		do   func(t *testing.T, s *Store)
		drop bool
	}{
		{"title only update", func(t *testing.T, s *Store) {
			mustPut(t, s, PutRequest{Key: "r-rule", Update: true, Title: &title, Gate: DefaultGate(), Scopes: []string{"global"}, Kind: Rule})
		}, true},
		{"retext", func(t *testing.T, s *Store) {
			if _, err := s.Retext(ctx, []Text{{Key: "r-rule", Title: "Rule title", Body: "english body"}}); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"retext with the same text", func(t *testing.T, s *Store) {
			if _, err := s.Retext(ctx, []Text{{Key: "r-rule", Title: "Rule title", Body: "rule body about pushing"}}); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"upsert with another body", func(t *testing.T, s *Store) {
			r, _ := s.Get(ctx, "r-rule")
			r.Body = "imported again with other words"
			if _, err := s.Upsert(ctx, []Record{r}, nil, "reimport"); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"upsert with the same text", func(t *testing.T, s *Store) {
			r, _ := s.Get(ctx, "r-rule")
			if _, err := s.Upsert(ctx, []Record{r}, nil, "reimport"); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"retire", func(t *testing.T, s *Store) {
			if _, err := s.Retire(ctx, "r-rule", "gone"); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"verify", func(t *testing.T, s *Store) {
			if _, err := s.Verify(ctx, "r-rule", "checked"); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := vecSeed(t)
			if _, _, err := s.PutVectors(ctx, "m", "D: ", rowsFor(t, s, "m", "D: ", "r-rule", "r-host")); err != nil {
				t.Fatal(err)
			}
			c.do(t, s)
			info, _ := s.VectorInfo(ctx, "m")
			_, has := info["r-rule"]
			if has == c.drop {
				t.Errorf("vector present %v, want %v", has, !c.drop)
			}
			if _, ok := info["r-host"]; !ok {
				t.Error("the vector of an untouched record is gone")
			}
			if has {
				r, _ := s.Get(ctx, "r-rule")
				if info["r-rule"].SHA != TextSHA(DocText("D: ", r)) {
					t.Error("a kept vector no longer matches the record text")
				}
			}
		})
	}
}
