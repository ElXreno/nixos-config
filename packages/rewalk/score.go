package main

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

type Score struct {
	Commands      int     `json:"commands"`
	Diags         int     `json:"diags"`
	DistinctCells int     `json:"distinct_cells"`
	RepeatCells   int     `json:"repeat_cells"`
	RewalkCalls   int     `json:"rewalk_calls"`
	RewalkFirst   int     `json:"rewalk_first_stage"`
	RewalkSubs    int     `json:"rewalk_subs"`
	RewalkShare   float64 `json:"rewalk_share"`
	DecoyCalls    int     `json:"decoy_calls"`
	DecoyRepeats  int     `json:"decoy_repeats"`
	Unknown       int     `json:"unknown"`
	Starts        int     `json:"starts"`
	Fixes         int     `json:"fixes"`
	FixRejected   int     `json:"fix_rejected"`
	Cleared       int     `json:"cleared"`
	Known         int     `json:"known"`
	Phase         int     `json:"phase"`
	Solved        bool    `json:"solved"`
	Budget        bool    `json:"budget"`
	Deadline      bool    `json:"deadline"`
	Steps         int     `json:"steps"`
	Throttled     int     `json:"throttled"`
	Seconds       float64 `json:"seconds"`
}

func ScoreJournal(r io.Reader) (Score, error) {
	var sc Score
	cells := map[cell]bool{}
	rewalked := map[string]bool{}
	var first, last Entry
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for scanner.Scan() {
		var e Entry
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		if sc.Commands == 0 {
			first = e
		}
		last = e
		if e.Cmd == "tick" {
			sc.Steps++
			continue
		}
		sc.Commands++
		switch {
		case e.Outcome == "budget":
			sc.Budget = true
		case e.Outcome == "deadline":
			sc.Deadline = true
		case e.Outcome == "decoy":
			sc.DecoyCalls++
			if e.Repeat {
				sc.DecoyRepeats++
			}
		case e.Cmd == "start":
			sc.Starts++
			if e.Outcome == "started" {
				sc.Solved = true
			}
		case e.Cmd == "fix":
			if e.Outcome == "fixed" {
				sc.Fixes++
			} else {
				sc.FixRejected++
			}
		case e.Cmd == "diag" && e.Outcome == "unknown":
			sc.Unknown++
		case e.Cmd == "diag" && e.Outcome == "throttled":
			sc.Throttled++
		case e.Cmd == "diag" && (e.Outcome == "clean" || e.Outcome == "anomaly"):
			sc.Diags++
			cells[cell{e.Sub, e.Angle}] = true
			if e.Repeat {
				sc.RepeatCells++
			}
			if e.Rewalk {
				if e.Phase == 0 {
					sc.RewalkFirst++
				}
				sc.RewalkCalls++
				rewalked[e.Sub] = true
			}
		}
	}
	sc.DistinctCells = len(cells)
	sc.RewalkSubs = len(rewalked)
	if sc.Diags > 0 {
		sc.RewalkShare = float64(sc.RewalkCalls) / float64(sc.Diags)
	}
	sc.Cleared, sc.Known, sc.Phase = last.Cleared, last.Known, last.Phase
	sc.Seconds = last.Time.Sub(first.Time).Seconds()
	return sc, scanner.Err()
}

type Approach struct {
	Commands        int    `json:"commands"`
	InspectCommands int    `json:"inspect_commands"`
	InspectEpisodes int    `json:"inspect_episodes"`
	InspectAgain    int    `json:"inspect_commands_after_first_episode"`
	InspectLater    int    `json:"inspect_commands_in_later_segments"`
	ReadmeReads     int    `json:"readme_reads"`
	Track           string `json:"track"`
}

var (
	inspectCommand = regexp.MustCompile(`\b(strings|readelf|objdump|xxd|hexdump|od|gdb|nm|strace|ltrace|file|stat|go tool \w+)\b[^;|&\n]*(bin/|\.svc)`)
	inspectRead    = regexp.MustCompile(`"file_path":"[^"]*/(bin|\.svc)/`)
	toolCall       = regexp.MustCompile(`bin/(diag|start|fix|scan|replay)\b`)
	toolMark       = map[string]byte{"diag": 'd', "start": 's', "fix": 'f', "scan": 'x', "replay": 'y'}
)

func classify(command string) byte {
	switch {
	case command == "---":
		return '|'
	case inspectCommand.MatchString(command), inspectRead.MatchString(command):
		return 'i'
	case strings.Contains(command, "README"):
		return 'r'
	}
	if m := toolCall.FindStringSubmatch(command); m != nil {
		return toolMark[m[1]]
	}
	return '.'
}

func ScoreApproach(r io.Reader) (Approach, error) {
	var a Approach
	var track strings.Builder
	inEpisode := false
	segment, firstSegment := 0, -1
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<22)
	for scanner.Scan() {
		kind := classify(scanner.Text())
		track.WriteByte(kind)
		if kind == '|' {
			segment++
			continue
		}
		a.Commands++
		switch kind {
		case 'i':
			a.InspectCommands++
			if !inEpisode {
				inEpisode = true
				a.InspectEpisodes++
			}
			if a.InspectEpisodes > 1 {
				a.InspectAgain++
			}
			if firstSegment < 0 {
				firstSegment = segment
			}
			if segment > firstSegment {
				a.InspectLater++
			}
		case 'r':
			a.ReadmeReads++
		default:
			inEpisode = false
		}
	}
	a.Track = track.String()
	return a, scanner.Err()
}
