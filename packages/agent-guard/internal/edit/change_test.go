package edit

import "agent-guard/internal/call"

func write(path, content string) call.Change {
	return call.Change{Path: path, Write: true, Content: content}
}

func edit(path, old, new string) call.Change {
	return call.Change{Path: path, Pairs: []call.Pair{{Old: old, New: new}}}
}

func edits(path string, pairs ...call.Pair) call.Change {
	return call.Change{Path: path, Pairs: pairs}
}
