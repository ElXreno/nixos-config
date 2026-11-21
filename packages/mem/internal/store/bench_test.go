package store

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

var benchWords = strings.Fields(`nix build switch deploy push commit fork remote kernel zfs readahead arc folio
compaction smok kurwa bimba grate tailscale wireguard udp buffer socket port 8097 8098 model strata freetoken
qwen deepseek expert predictor vram dram decode prefill cache attic ncps narinfo store path flake input overlay
module option home manager agent guard hook memory record supersede gate duplicate scope trigger
сборка ядро память модель громкость динамик батарея прошивка кулер плата биос андервольт фриз загрузка
пуш коммит форк ветка патч драйвер монитор дисплей яркость звук блютуз наушники сеть туннель роутер`)

var benchVocab = func() []string {
	syl := []string{"ka", "ro", "mi", "zu", "te", "lo", "ban", "pri", "sto", "vel", "ни", "ко", "ра", "ст", "мо", "ду"}
	r := rand.New(rand.NewPCG(9, 9))
	out := append([]string{}, benchWords...)
	for len(out) < 20000 {
		var b strings.Builder
		for j := 0; j < 2+r.IntN(3); j++ {
			b.WriteString(syl[r.IntN(len(syl))])
		}
		out = append(out, b.String())
	}
	return out
}()

func benchText(r *rand.Rand, n int) string {
	z := rand.NewZipf(r, 1.1, 2, uint64(len(benchVocab)-1))
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(benchVocab[z.Uint64()])
	}
	return b.String()
}

func benchStore(b *testing.B, n int) *Store {
	b.Helper()
	ctx := context.Background()
	s, err := Open(ctx, b.TempDir()+"/mem.db", Options{})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	r := rand.New(rand.NewPCG(1, 2))
	recs := make([]Record, n)
	for i := range recs {
		kind := Kinds[i%len(Kinds)]
		recs[i] = Record{
			Key:    fmt.Sprintf("rec-%05d", i),
			Kind:   kind,
			Scopes: []string{GlobalScope, fmt.Sprintf("project:p%d", i%5)},
			Title:  benchText(r, 6),
			Body:   benchText(r, 150),
		}
	}
	if _, err := s.Upsert(ctx, recs, nil, "bench"); err != nil {
		b.Fatal(err)
	}
	if err := s.Optimize(ctx); err != nil {
		b.Fatal(err)
	}
	return s
}

func BenchmarkRecall5000(b *testing.B) {
	s := benchStore(b, 5000)
	r := rand.New(rand.NewPCG(3, 4))
	prompt := benchText(r, 300)
	for _, m := range []Mode{Fused, WordsOnly, TrigramOnly} {
		q := Query{Text: prompt, Limit: 4, ExcludeKinds: []Kind{Log}, Scopes: []string{GlobalScope, "project:p1", "host:smok"}, Mode: m}
		b.Run(m.String(), func(b *testing.B) {
			for b.Loop() {
				if _, err := s.Search(context.Background(), q); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkGatePut5000(b *testing.B) {
	s := benchStore(b, 5000)
	r := rand.New(rand.NewPCG(5, 6))
	i := 0
	b.ResetTimer()
	for b.Loop() {
		i++
		body := benchText(r, 150)
		_, err := s.Put(context.Background(), PutRequest{Key: fmt.Sprintf("new-%d", i), Kind: Rule, Scopes: []string{GlobalScope}, Body: &body,
			Gate: DefaultGate()})
		var ge *GateError
		if err != nil && !errors.As(err, &ge) {
			b.Fatal(err)
		}
	}
}
