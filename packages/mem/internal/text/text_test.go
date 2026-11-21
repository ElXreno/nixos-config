package text

import (
	"reflect"
	"testing"
)

func TestStem(t *testing.T) {
	cases := []struct{ in, want string }{
		{"сборка", "сборк"},
		{"сборки", "сборк"},
		{"сборку", "сборк"},
		{"пушить", "пуш"},
		{"пушим", "пуш"},
		{"разрешения", "разрешен"},
		{"разрешение", "разрешен"},
		{"коммита", "коммит"},
		{"коммитов", "коммит"},
		{"явного", "явн"},
		{"явное", "явн"},
		{"умолчанию", "умолчан"},
		{"умолчания", "умолчан"},
		{"громкость", "громкост"},
		{"громкости", "громкост"},
		{"динамиках", "динамик"},
		{"запускать", "запуска"},
		{"запуская", "запуск"},
		{"push", "push"},
		{"pushing", "push"},
		{"pushed", "push"},
		{"pushes", "push"},
		{"builds", "build"},
		{"building", "build"},
		{"committed", "commit"},
		{"commits", "commit"},
		{"running", "run"},
		{"stopped", "stop"},
		{"copies", "cop"},
		{"copied", "cop"},
		{"queries", "quer"},
		{"query", "quer"},
		{"copy", "cop"},
		{"deploy", "deploy"},
		{"deployed", "deploy"},
		{"explicitly", "explicit"},
		{"8097", "8097"},
		{"0xdf", "0xdf"},
		{"nvme0n1", "nvme0n1"},
	}
	for _, c := range cases {
		if got := Stem(c.in); got != c.want {
			t.Errorf("Stem(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Don't push 127.0.0.1:8097", []string{"don", "t", "push", "127", "0", "0", "1", "8097"}},
		{"Ёлка СБОРКА feedback_no_push", []string{"елка", "сборка", "feedback", "no", "push"}},
		{"Apex 0xD2 (нельзя)", []string{"apex", "0xd2", "нельзя"}},
	}
	for _, c := range cases {
		if got := Tokens(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Tokens(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Never push to the remote by default", []string{"push", "remot", "default"}},
		{"Не пушить по умолчанию, не пушить", []string{"пуш", "умолчан"}},
		{"the of и в", nil},
	}
	for _, c := range cases {
		if got := Terms(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Terms(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSimilarityBounds(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"push remote default", "push remote default", 1},
		{"push remote default", "сборка ядра", 0},
		{"", "push", 0},
	}
	for _, c := range cases {
		if got := Similarity(c.a, c.b); got != c.want {
			t.Errorf("Similarity(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestStemIsPrefix(t *testing.T) {
	words := Tokens(`Никогда не пушить в nixos-config по умолчанию; после коммита ждать явного разрешения.
Громкость динамиков не трогать без спроса, даже с сохранением итога. Сборки запускать в фоне,
длинные билды не гонять на батарее. Never push by default: after committing, wait for explicit
authorization. Copies, copied, queries, dependencies, running, stopped, builds, deployed, explicitly,
classes, boxes, wishes, matches, caresses, agreed, ponies, studied.`)
	for _, w := range words {
		if s := Stem(w); len(s) > len(w) || w[:len(s)] != s {
			t.Errorf("Stem(%q) = %q is not a prefix", w, s)
		}
	}
}

func TestCoverage(t *testing.T) {
	cases := []struct {
		query, doc string
		want       float64
	}{
		{"pushing commits", "Do not git push after a commit.", 1},
		{"BufferSize udp", "Tailscale socketBufferSize patched.", 0.5},
		{"сборку на батарее", "Сборки не запускать на батарее.", 1},
		{"ёлка", "Елка в углу", 1},
		{"5g", "a 5g modem", 1},
		{"5g", "a 5gb disk", 0},
		{"the of", "anything", 0},
	}
	for _, c := range cases {
		if got := Coverage(Terms(c.query), c.doc); got != c.want {
			t.Errorf("Coverage(%q, %q) = %v, want %v", c.query, c.doc, got, c.want)
		}
	}
}

func TestLooseTermsKeepDeonticWords(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Never push to the remote by default", []string{"never", "push", "remot", "default"}},
		{"Не пушить по умолчанию, не пушить", []string{"не", "пуш", "умолчан"}},
		{"you must always use only the cache", []string{"must", "alway", "use", "onl", "cach"}},
		{"нужно только всегда без спроса", []string{"нужн", "тольк", "всегд", "без", "спрос"}},
		{"Don't push", []string{"don", "push"}},
		{"the of и в", nil},
	}
	for _, c := range cases {
		got := LooseTerms(c.in)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("LooseTerms(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDeonticWordsStayOutOfTheGate(t *testing.T) {
	for _, w := range []string{"never", "no", "not", "nor", "cannot", "without", "don", "must", "should", "always", "only", "не", "нет", "ни", "нельзя", "никогда", "ничего", "без", "надо", "можно", "только", "всегда"} {
		if !IsStopword(w) {
			t.Errorf("IsStopword(%q) = false", w)
		}
		if got := Terms(w); got != nil {
			t.Errorf("Terms(%q) = %q, want nil", w, got)
		}
		if got := LooseTerms(w); len(got) != 1 {
			t.Errorf("LooseTerms(%q) = %q, want one term", w, got)
		}
	}
	for _, w := range []string{"the", "и", "для", "would", "очень", "в", "of"} {
		if got := LooseTerms(w); got != nil {
			t.Errorf("LooseTerms(%q) = %q, want nil", w, got)
		}
	}
	if got := NewSet("и в на нельзя never"); len(got) != 0 {
		t.Errorf("NewSet of stopwords and deontic words only = %v, want empty", got)
	}
}

func TestRawTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"the of и в", []string{"the", "of", "и", "в"}},
		{"The the ТО то", []string{"the", "то"}},
		{"и, в; на!", []string{"и", "в", "на"}},
		{"", nil},
		{"... --- !!!", nil},
	}
	for _, c := range cases {
		got := RawTerms(c.in)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("RawTerms(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
