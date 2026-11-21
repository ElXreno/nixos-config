package store

import (
	"errors"
	"testing"
)

func TestRetext(t *testing.T) {
	s, c := newStore(t)
	mustPut(t, s, spec{key: "a-rule", title: "Старый заголовок", body: "Старое тело записи про кулер"}.req())
	mustPut(t, s, spec{key: "b-rule", title: "Второй", body: "Второе тело про пуш", core: true}.req())
	c.advance(1)
	before, err := s.Get(t.Context(), "a-rule")
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.Retext(t.Context(), []Text{{Key: "a-rule", Title: " New title ", Body: "New body about the cooler\r\n\n"}})
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	after, _ := s.Get(t.Context(), "a-rule")
	if after.Title != "New title" || after.Body != "New body about the cooler" {
		t.Errorf("%q %q", after.Title, after.Body)
	}
	if after.Version != before.Version || !after.UpdatedAt.Equal(before.UpdatedAt) || after.Status != before.Status {
		t.Errorf("revision bookkeeping moved: %+v vs %+v", after, before)
	}
	if h, _ := s.History(t.Context(), "a-rule"); len(h) != 0 {
		t.Errorf("history %v", h)
	}
	hits, err := s.Search(t.Context(), Query{Text: "cooler", Mode: WordsOnly})
	if err != nil || len(hits) != 1 || hits[0].Record.Key != "a-rule" {
		t.Errorf("new words: %v %v", hits, err)
	}
	if hits, _ := s.Search(t.Context(), Query{Text: "кулер", Mode: WordsOnly}); len(hits) != 0 {
		t.Errorf("old words still indexed: %v", hits)
	}
	if hits, _ := s.Search(t.Context(), Query{Text: "cooler", Mode: TrigramOnly}); len(hits) != 1 {
		t.Errorf("trigram index: %v", hits)
	}
	if _, err := s.Retext(t.Context(), []Text{{Key: "b-rule", Title: "t", Body: "ok"}, {Key: "a-rule", Title: "t", Body: " \n"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty body: %v", err)
	}
	if b, _ := s.Get(t.Context(), "b-rule"); b.Body != "Второе тело про пуш" {
		t.Errorf("failed batch was not rolled back: %q", b.Body)
	}
	if _, err := s.Retext(t.Context(), []Text{{Key: "nope", Title: "t", Body: "b"}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown key: %v", err)
	}
}
