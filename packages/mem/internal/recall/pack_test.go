package recall

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"mem/internal/store"
)

func TestPackTruncates(t *testing.T) {
	long := store.Record{Key: "big", Kind: store.Fact, Body: strings.Repeat("word ", 1500)}
	small := store.Record{Key: "small", Kind: store.Rule, Body: "short body"}
	p := Pack("header\n", []store.Record{long, small}, "", 2500)
	if n := len([]rune(p.Text)); n > 2500 {
		t.Errorf("packed %d chars over the 2500 budget", n)
	}
	if len(p.Keys) != 2 || p.Keys[0] != "big" || len(p.Dropped) != 0 {
		t.Errorf("keys %v dropped %v", p.Keys, p.Dropped)
	}
	for _, w := range []string{"`mem get big` prints the whole record", "`small` [rule"} {
		if !strings.Contains(p.Text, w) {
			t.Errorf("packed text lacks %q", w)
		}
	}
	p = Pack("header\n", []store.Record{small, long}, "", 200)
	if len(p.Keys) != 1 || p.Keys[0] != "small" || len(p.Dropped) != 1 || len([]rune(p.Text)) > 200 {
		t.Errorf("tight budget: keys %v dropped %v len %d", p.Keys, p.Dropped, len([]rune(p.Text)))
	}
	if p := Pack("header\n", nil, "", 2500); p.Text != "" || p.Keys != nil {
		t.Errorf("empty pack: %q %v", p.Text, p.Keys)
	}
}

func lines(prefix string, n, width int) string {
	var b strings.Builder
	for i := range n {
		line := prefix + " line " + strings.Repeat("x", 3) + " "
		for len([]rune(line)) < width {
			line += "filler "
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(line))
	}
	return b.String()
}

func TestPackKeepsRankOrder(t *testing.T) {
	long := store.Record{Key: "long", Kind: store.Fact, Title: "Long record", Body: lines("long", 40, 100)}
	medium := store.Record{Key: "medium", Kind: store.Fact, Title: "Medium record", Body: lines("medium", 22, 100)}
	p := Pack("header\n", []store.Record{long, medium}, "", 2500)
	if len([]rune(p.Text)) > 2500 {
		t.Errorf("packed %d chars over the budget", len([]rune(p.Text)))
	}
	if len(p.Keys) != 2 || p.Keys[0] != "long" || p.Keys[1] != "medium" || len(p.Dropped) != 0 {
		t.Fatalf("keys %v dropped %v: the top record gave way to a lower one", p.Keys, p.Dropped)
	}
	if i, j := strings.Index(p.Text, "`long`"), strings.Index(p.Text, "`medium`"); i < 0 || j < i {
		t.Errorf("blocks out of rank order")
	}
}

func TestPackFairShare(t *testing.T) {
	a := store.Record{Key: "a", Kind: store.Fact, Body: lines("alpha", 20, 100)}
	b := store.Record{Key: "b", Kind: store.Fact, Body: lines("beta", 20, 100)}
	short := store.Record{Key: "short", Kind: store.Rule, Body: "short body"}
	p := Pack("header\n", []store.Record{a, b, short}, "", 2500)
	if len(p.Keys) != 3 || len(p.Dropped) != 0 {
		t.Fatalf("keys %v dropped %v", p.Keys, p.Dropped)
	}
	blocks := strings.Split(strings.TrimPrefix(p.Text, "header\n"), "\n- `")
	if len(blocks) != 3 {
		t.Fatalf("%d blocks", len(blocks))
	}
	na, nb := len([]rune(blocks[0])), len([]rune(blocks[1]))
	if na > 1300 || nb > 1300 || na < 1000 || nb < 1000 {
		t.Errorf("blocks of %d and %d chars, want a fair share of about 1240 each", na, nb)
	}
	if !strings.Contains(p.Text, "`short` [rule") || !strings.Contains(p.Text, "short body") {
		t.Errorf("short record not whole:\n%s", p.Text)
	}
}

func TestPackExcerpt(t *testing.T) {
	body := strings.Split(lines("intro", 30, 100), "\n")
	body[21] = "zfs_arc_max caps the ARC on BIMBA at 8 GB since the October rebuild."
	body[25] = "A single paragraph that runs long. " + strings.Repeat("Unrelated words fill this sentence. ", 12) + "Then ARC tuning on BIMBA comes up again. " + strings.Repeat("More filler here. ", 10)
	long := store.Record{Key: "arc", Kind: store.Fact, Title: "ARC notes", Body: strings.Join(body, "\n")}
	other := store.Record{Key: "other", Kind: store.Fact, Body: lines("other", 30, 100)}
	p := Pack("header\n", []store.Record{long, other}, "what is zfs_arc_max on BIMBA", 2500)
	if len(p.Keys) != 2 || len(p.Cut) != 2 || len([]rune(p.Text)) > 2500 {
		t.Fatalf("keys %v cut %v len %d", p.Keys, p.Cut, len([]rune(p.Text)))
	}
	block := p.Text[:strings.Index(p.Text, "- `other`")]
	for _, w := range []string{"- `arc` [fact, as of", "ARC notes. intro line", "zfs_arc_max caps the ARC on BIMBA at 8 GB",
		"\n  ...\n", "Then ARC tuning on BIMBA", "`mem get arc` prints the whole record"} {
		if !strings.Contains(block, w) {
			t.Errorf("excerpt lacks %q:\n%s", w, block)
		}
	}
	if strings.Contains(block, "intro line xxx filler filler filler filler filler filler filler filler filler filler filler filler\n  ...\n  ...") {
		t.Errorf("doubled gap marker:\n%s", block)
	}
	one := store.Record{Key: "one", Kind: store.Fact, Body: strings.Repeat("word ", 1000) + "needle at the end"}
	p = Pack("header\n", []store.Record{one, other}, "needle", 1200)
	if !strings.Contains(p.Text, "needle at the end") || !strings.Contains(p.Text, "- `one` [fact") {
		t.Errorf("one-line body lost its match:\n%s", p.Text)
	}
}

var completeLink = regexp.MustCompile(`\[[^\[\]\n]*\]\([^()\s]*\)`)

func TestPackKeepsLinksWhole(t *testing.T) {
	var body []string
	for i := range 30 {
		body = append(body, fmt.Sprintf("Bucket %02d policy: see [rate limits](https://help.mega.io/megas4/setup-guides/rate-limits-and-quotas-for-s4) "+
			"and [pricing tiers](https://mega.io/objectstorage/pricing-tiers-for-s4-storage) before tuning the uploader of bucket %02d.", i, i))
	}
	links := store.Record{Key: "links", Kind: store.Fact, Title: "S4 links", Body: strings.Join(body, "\n")}
	one := store.Record{Key: "one", Kind: store.Fact, Body: strings.ReplaceAll(strings.Join(body, " "), "\n", " ")}
	run := store.Record{Key: "run", Kind: store.Fact, Body: strings.ReplaceAll(strings.Join(body, " - "), ". -", " -")}
	other := store.Record{Key: "other", Kind: store.Fact, Body: lines("other", 30, 100)}
	broken := 0
	for budget := 500; budget <= 2500; budget += 23 {
		for _, q := range []string{"", "tuning the uploader of bucket 17", "pricing tiers bucket 23"} {
			for _, recs := range [][]store.Record{{links, other}, {one, other}, {run, other}} {
				p := Pack("header\n", recs, q, budget)
				rest := completeLink.ReplaceAllString(p.Text, "")
				for _, frag := range []string{"](", "help.mega", "objectstorage", "[rate", "[pricing", "rate limits", "pricing tiers"} {
					if strings.Contains(rest, frag) {
						if broken < 3 {
							t.Errorf("budget %d query %q: link split, %q outside a whole link:\n%s", budget, q, frag, p.Text)
						}
						broken++
						break
					}
				}
			}
		}
	}
	if broken > 0 {
		t.Errorf("%d packs split a link", broken)
	}
}

func TestPackDropsFromTheBottom(t *testing.T) {
	var recs []store.Record
	for _, k := range []string{"r1", "r2", "r3", "r4"} {
		recs = append(recs, store.Record{Key: k, Kind: store.Fact, Body: lines(k, 20, 100)})
	}
	p := Pack("header\n", recs, "", 1000)
	if len(p.Keys) != 3 || p.Keys[0] != "r1" || p.Keys[2] != "r3" || len(p.Dropped) != 1 || p.Dropped[0] != "r4" {
		t.Errorf("keys %v dropped %v, want the lowest-ranked record dropped once shares fall under %d", p.Keys, p.Dropped, MinTruncated)
	}
	if n := len([]rune(p.Text)); n > 1000 {
		t.Errorf("%d chars over the budget", n)
	}
}
