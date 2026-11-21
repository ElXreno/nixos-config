package text

type ending struct {
	s     []rune
	after bool
}

type endingSet []ending

func endings(afterAYa []string, plain []string) endingSet {
	var set endingSet
	for _, s := range afterAYa {
		set = append(set, ending{s: []rune(s), after: true})
	}
	for _, s := range plain {
		set = append(set, ending{s: []rune(s)})
	}
	return set
}

var (
	ruPerfectiveGerund = endings(
		[]string{"в", "вши", "вшись"},
		[]string{"ив", "ивши", "ившись", "ыв", "ывши", "ывшись"},
	)
	ruAdjective = endings(nil, []string{
		"ее", "ие", "ые", "ое", "ими", "ыми", "ей", "ий", "ый", "ой", "ем", "им", "ым", "ом",
		"его", "ого", "ему", "ому", "их", "ых", "ую", "юю", "ая", "яя", "ою", "ею",
	})
	ruParticiple = endings(
		[]string{"ем", "нн", "вш", "ющ", "щ"},
		[]string{"ивш", "ывш", "ующ"},
	)
	ruReflexive = endings(nil, []string{"ся", "сь"})
	ruVerb      = endings(
		[]string{"ла", "на", "ете", "йте", "ли", "й", "л", "ем", "н", "ло", "но", "ет", "ют", "ны", "ть", "ешь", "нно"},
		[]string{
			"ила", "ыла", "ена", "ейте", "уйте", "ите", "или", "ыли", "ей", "уй", "ил", "ыл", "им", "ым", "ен",
			"ило", "ыло", "ено", "ят", "ует", "уют", "ит", "ыт", "ены", "ить", "ыть", "ишь", "ую", "ю",
		},
	)
	ruNoun = endings(nil, []string{
		"а", "ев", "ов", "ие", "ье", "е", "иями", "ями", "ами", "еи", "ии", "и", "ией", "ей", "ой", "ий", "й",
		"иям", "ям", "ием", "ем", "ам", "ом", "о", "у", "ах", "иях", "ях", "ы", "ь", "ию", "ью", "ю", "ия", "ья", "я",
	})
	ruSuperlative  = endings(nil, []string{"ейше", "ейш"})
	ruDerivational = endings(nil, []string{"ость", "ост"})
)

func isRuVowel(r rune) bool {
	switch r {
	case 'а', 'е', 'и', 'о', 'у', 'ы', 'э', 'ю', 'я':
		return true
	}
	return false
}

func hasSuffix(w, s []rune) bool {
	if len(s) > len(w) {
		return false
	}
	off := len(w) - len(s)
	for i, r := range s {
		if w[off+i] != r {
			return false
		}
	}
	return true
}

func (set endingSet) strip(w []rune, limit int) ([]rune, bool) {
	best := -1
	for i, e := range set {
		if !hasSuffix(w, e.s) || len(w)-len(e.s) < limit {
			continue
		}
		if best < 0 || len(e.s) > len(set[best].s) {
			best = i
		}
	}
	if best < 0 {
		return w, false
	}
	e := set[best]
	start := len(w) - len(e.s)
	if e.after {
		if start-1 < limit {
			return w, false
		}
		if p := w[start-1]; p != 'а' && p != 'я' {
			return w, false
		}
	}
	return w[:start], true
}

func ruRegion(w []rune, from int) int {
	for i := from + 1; i < len(w); i++ {
		if !isRuVowel(w[i]) && isRuVowel(w[i-1]) {
			return i + 1
		}
	}
	return len(w)
}

func stemRu(w []rune) []rune {
	rv := len(w)
	for i, r := range w {
		if isRuVowel(r) {
			rv = i + 1
			break
		}
	}
	if rv >= len(w) {
		return w
	}
	r2 := ruRegion(w, ruRegion(w, 0))

	if s, ok := ruPerfectiveGerund.strip(w, rv); ok {
		w = s
	} else {
		w, _ = ruReflexive.strip(w, rv)
		if s, ok := ruAdjective.strip(w, rv); ok {
			w = s
			w, _ = ruParticiple.strip(w, rv)
		} else if s, ok := ruVerb.strip(w, rv); ok {
			w = s
		} else {
			w, _ = ruNoun.strip(w, rv)
		}
	}

	if len(w) > rv && w[len(w)-1] == 'и' {
		w = w[:len(w)-1]
	}

	if s, ok := ruDerivational.strip(w, r2); ok {
		w = s
	}

	switch {
	case hasSuffix(w, []rune("нн")) && len(w)-2 >= rv:
		w = w[:len(w)-1]
	default:
		if s, ok := ruSuperlative.strip(w, rv); ok {
			w = s
			if hasSuffix(w, []rune("нн")) && len(w)-2 >= rv {
				w = w[:len(w)-1]
			}
		} else if len(w) > rv && w[len(w)-1] == 'ь' {
			w = w[:len(w)-1]
		}
	}
	return w
}
