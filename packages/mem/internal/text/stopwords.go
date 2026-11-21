package text

func setOf(groups ...[]string) map[string]bool {
	m := map[string]bool{}
	for _, g := range groups {
		for _, w := range g {
			m[w] = true
		}
	}
	return m
}

var functionWords = []string{
	"a", "about", "after", "again", "all", "also", "am", "an", "and", "any", "are", "as", "at",
	"be", "been", "before", "being", "both", "but", "by", "can", "could", "did", "do", "does",
	"doing", "done", "each", "else", "even", "ever", "every", "for", "from", "had", "has", "have",
	"having", "he", "her", "here", "him", "his", "how", "i", "if", "in", "into", "is", "it", "its", "just",
	"may", "me", "might", "more", "most", "my", "now", "of", "off",
	"on", "once", "or", "other", "our", "out", "over", "own", "same", "she", "so",
	"some", "such", "than", "that", "the", "their", "them", "then", "there", "these", "they", "this",
	"those", "through", "to", "too", "under", "until", "up", "us", "very", "was", "we", "were", "what",
	"when", "where", "which", "while", "who", "why", "will", "with", "would", "you", "your",
	"а", "более", "больше", "будет", "будто", "бы", "был", "была", "были", "было", "быть", "в",
	"вам", "вас", "ведь", "весь", "во", "вот", "все", "всего", "всех", "всю", "вы", "где",
	"да", "даже", "для", "до", "его", "ее", "её", "ей", "ему", "если", "есть", "еще", "ещё", "же", "за",
	"здесь", "и", "из", "или", "им", "их", "к", "как", "какой", "когда", "кто", "ли", "либо", "между",
	"меня", "мне", "много", "может", "мой", "моя", "мы", "на", "над", "наш",
	"него", "нее", "неё", "ним", "них", "но", "ну", "о",
	"об", "один", "он", "она", "они", "оно", "от", "очень", "по", "под", "после", "потом", "потому",
	"при", "про", "раз", "с", "сам", "свой", "себе", "себя", "со", "так", "также", "такой", "там", "те",
	"тем", "то", "того", "тоже", "том", "тот", "тут", "ты", "у", "уже", "чем", "через", "что",
	"чтоб", "чтобы", "чуть", "эта", "эти", "это", "этого", "этой", "этом", "этот", "эту", "я",
}

var negationWords = []string{
	"never", "no", "nor", "not", "cannot", "without", "don",
	"не", "нет", "ни", "нельзя", "никогда", "ничего", "без",
}

var modalWords = []string{
	"must", "should", "always", "only",
	"надо", "можно", "всегда", "только",
}

var stopwords = setOf(functionWords, negationWords, modalWords)

var searchStopwords = setOf(functionWords)

func IsStopword(token string) bool {
	return stopwords[token]
}
