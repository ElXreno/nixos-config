package text

import "strings"

func isEnVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

func hasEnVowel(s string) bool {
	for i := 0; i < len(s); i++ {
		if isEnVowel(s[i]) {
			return true
		}
	}
	return false
}

func undouble(s string) string {
	n := len(s)
	if n < 3 || s[n-1] != s[n-2] || isEnVowel(s[n-1]) {
		return s
	}
	switch s[n-1] {
	case 'l', 's', 'z':
		return s
	}
	return s[:n-1]
}

func stemEn(w string) string {
	if len(w) < 4 {
		return w
	}
	switch {
	case strings.HasSuffix(w, "sses"):
		w = w[:len(w)-2]
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		w = w[:len(w)-3]
	case strings.HasSuffix(w, "ches"), strings.HasSuffix(w, "shes"), strings.HasSuffix(w, "xes"), strings.HasSuffix(w, "zes"):
		w = w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "is"):
		w = w[:len(w)-1]
	}
	switch {
	case strings.HasSuffix(w, "eed"):
	case strings.HasSuffix(w, "ied") && len(w) > 4:
		w = w[:len(w)-3]
	case strings.HasSuffix(w, "ing") && len(w) > 5 && hasEnVowel(w[:len(w)-3]):
		w = undouble(w[:len(w)-3])
	case strings.HasSuffix(w, "ed") && len(w) > 4 && hasEnVowel(w[:len(w)-2]):
		w = undouble(w[:len(w)-2])
	case strings.HasSuffix(w, "ly") && len(w) > 5:
		w = w[:len(w)-2]
	}
	if len(w) > 4 && strings.HasSuffix(w, "e") && !strings.HasSuffix(w, "ee") {
		w = w[:len(w)-1]
	}
	if len(w) > 3 && strings.HasSuffix(w, "y") && !isEnVowel(w[len(w)-2]) {
		w = w[:len(w)-1]
	}
	return w
}
