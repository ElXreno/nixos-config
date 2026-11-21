package recall

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"mem/internal/store"
)

func benchStore(b *testing.B, n, dim int) *store.Store {
	b.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(b.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	r := rand.New(rand.NewSource(3))
	var recs []store.Record
	for i := 0; i < n; i++ {
		recs = append(recs, store.Record{Key: fmt.Sprintf("rec-%04d", i), Kind: store.Fact, Scopes: []string{"global"}, Title: fmt.Sprintf("T %d", i), Body: fmt.Sprintf("body %d", i)})
	}
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		b.Fatal(err)
	}
	var rows []store.VecRow
	for _, rec := range recs {
		rows = append(rows, store.VecRow{Key: rec.Key, Vec: randomUnit(r, dim, false), TextSHA: store.TextSHA(store.DocText("D: ", rec))})
	}
	if _, _, err := s.PutVectors(ctx, "m", "D: ", rows); err != nil {
		b.Fatal(err)
	}
	return s
}

func BenchmarkLoadAndScore3268x1024(b *testing.B) {
	s := benchStore(b, 3268, 1024)
	q := randomUnit(rand.New(rand.NewSource(4)), 1024, false)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix, err := LoadIndex(ctx, s, "m", store.Query{Scopes: []string{"global"}})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := ix.Score(q); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScoreOnly3268x1024(b *testing.B) {
	s := benchStore(b, 3268, 1024)
	q := randomUnit(rand.New(rand.NewSource(4)), 1024, false)
	ix, err := LoadIndex(context.Background(), s, "m", store.Query{})
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ix.Score(q); err != nil {
			b.Fatal(err)
		}
	}
}
