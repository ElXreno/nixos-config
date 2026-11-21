package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	recSize     = 52
	offsetsSize = 32

	flagContiguousSources = 1 << 4
	flagSourceHashes      = 1 << 5

	encUTF8   = 0
	encLatin1 = 1
	encUTF16  = 2

	loaderJS = 1

	namePrefix = "/$bunfs/"
)

var (
	le           = binary.LittleEndian
	trailer      = []byte("\n---- Bun! ----\n")
	payloadLimit = uint64(math.MaxUint32)
)

type span struct{ off, n uint32 }

func (s span) end() uint64 { return uint64(s.off) + uint64(s.n) }

func readSpan(b []byte, o int) span {
	return span{le.Uint32(b[o:]), le.Uint32(b[o+4:])}
}

func putSpan(b []byte, o int, s span) {
	le.PutUint32(b[o:], s.off)
	le.PutUint32(b[o+4:], s.n)
}

type graph struct {
	payload      []byte
	offsetsStart int
	modules      span
	entry        uint32
	argv         span
	flags        uint32
	names        []string
	byName       map[string]int
}

func parseGraph(payload []byte) (*graph, error) {
	if len(payload) < offsetsSize+len(trailer) {
		return nil, fmt.Errorf("payload of %d bytes is too short", len(payload))
	}
	if !bytes.Equal(payload[len(payload)-len(trailer):], trailer) {
		return nil, fmt.Errorf("payload does not end with the standalone graph trailer")
	}
	os := len(payload) - len(trailer) - offsetsSize
	g := &graph{payload: payload, offsetsStart: os, byName: map[string]int{}}
	if bc := le.Uint64(payload[os:]); bc != uint64(os) {
		return nil, fmt.Errorf("byteCount %d differs from the Offsets start %d", bc, os)
	}
	g.modules = readSpan(payload, os+8)
	g.entry = le.Uint32(payload[os+16:])
	g.argv = readSpan(payload, os+20)
	g.flags = le.Uint32(payload[os+28:])
	if g.modules.n == 0 || g.modules.n%recSize != 0 || g.modules.end() > uint64(os) {
		return nil, fmt.Errorf("module table %+v is not a run of %d-byte records inside %d bytes", g.modules, recSize, os)
	}
	if g.argv.end() > uint64(os) {
		return nil, fmt.Errorf("compile argv %+v out of range", g.argv)
	}
	count := int(g.modules.n / recSize)
	if g.flags&flagSourceHashes != 0 && g.modules.end()+4*uint64(count) > uint64(os) {
		return nil, fmt.Errorf("source hash table of %d slots out of range", count)
	}
	if int(g.entry) >= count {
		return nil, fmt.Errorf("entry point %d out of %d modules", g.entry, count)
	}
	for i := 0; i < count; i++ {
		r := g.rec(i)
		for k := 0; k < 48; k += 8 {
			if readSpan(payload, r+k).end() > uint64(os) {
				return nil, fmt.Errorf("module %d: pointer at record byte %d out of range", i, k)
			}
		}
		if enc := payload[r+48]; enc > encUTF16 {
			return nil, fmt.Errorf("module %d: unknown encoding %d", i, enc)
		}
		ns := readSpan(payload, r)
		name := string(payload[ns.off:ns.end()])
		if !strings.HasPrefix(name, namePrefix) {
			return nil, fmt.Errorf("module %d: name %q lacks %s, record layout mismatch", i, name, namePrefix)
		}
		if _, dup := g.byName[name]; dup {
			return nil, fmt.Errorf("module name %s listed twice", name)
		}
		g.byName[name] = i
		g.names = append(g.names, name)
	}
	return g, nil
}

func (g *graph) rec(i int) int { return int(g.modules.off) + i*recSize }

func (g *graph) hashSlot(i int) int { return int(g.modules.end()) + 4*i }

func (g *graph) contents(i int) []byte {
	c := readSpan(g.payload, g.rec(i)+8)
	return g.payload[c.off:c.end()]
}

func latin1ToUTF8(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		out = utf8.AppendRune(out, rune(c))
	}
	return out
}

func (g *graph) source(i int) ([]byte, error) {
	raw := g.contents(i)
	switch enc := g.payload[g.rec(i)+48]; enc {
	case encUTF8:
		if !utf8.Valid(raw) {
			return nil, fmt.Errorf("tagged UTF-8 but holds invalid UTF-8")
		}
		return append([]byte(nil), raw...), nil
	case encLatin1:
		return latin1ToUTF8(raw), nil
	default:
		return nil, fmt.Errorf("encoding %d (UTF-16) is not supported", enc)
	}
}

type change struct {
	index    int
	module   string
	replaces int
	source   []byte
	oldLen   int
	appended bool
}

func applyReplacements(src []byte, module string, rs []replacement) ([]byte, error) {
	for i, rp := range rs {
		if rp.Old == "" {
			return nil, fmt.Errorf("%s: replacement %d has an empty old string", module, i)
		}
		if !utf8.ValidString(rp.New) || strings.IndexByte(rp.New, 0) >= 0 {
			return nil, fmt.Errorf("%s: replacement %d new text is not NUL-free UTF-8", module, i)
		}
		if n := bytes.Count(src, []byte(rp.Old)); n != 1 {
			return nil, fmt.Errorf("%s: replacement %d matches %d times, want 1", module, i, n)
		}
		src = bytes.Replace(src, []byte(rp.Old), []byte(rp.New), 1)
	}
	return src, nil
}

func (g *graph) patch(patches []modulePatch) ([]byte, []change, error) {
	var changes []change
	for _, p := range patches {
		i, ok := g.byName[p.Module]
		if !ok {
			return nil, nil, fmt.Errorf("module %s not in the graph", p.Module)
		}
		r := g.rec(i)
		if g.payload[r+49] != loaderJS {
			return nil, nil, fmt.Errorf("%s: loader %d is not JS", p.Module, g.payload[r+49])
		}
		src, err := g.source(i)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %v", p.Module, err)
		}
		out, err := applyReplacements(src, p.Module, p.Replace)
		if err != nil {
			return nil, nil, err
		}
		changes = append(changes, change{index: i, module: p.Module, replaces: len(p.Replace), source: out, oldLen: len(g.contents(i))})
	}
	np := append([]byte(nil), g.payload[:g.offsetsStart]...)
	appended := false
	for k := range changes {
		c := &changes[k]
		r := g.rec(c.index)
		cs := readSpan(np, r+8)
		if len(c.source) <= int(cs.n) {
			copy(np[cs.off:], c.source)
			copy(np[int(cs.off)+len(c.source):cs.end()], bytes.Repeat([]byte(" "), int(cs.n)-len(c.source)))
		} else {
			if uint64(len(np))+uint64(len(c.source))+1 > payloadLimit {
				return nil, nil, fmt.Errorf("%s: payload would exceed 4 GiB", c.module)
			}
			putSpan(np, r+8, span{uint32(len(np)), uint32(len(c.source))})
			np = append(np, c.source...)
			np = append(np, 0)
			c.appended = true
			appended = true
		}
		clear(np[r+16 : r+48])
		np[r+48] = encUTF8
		if g.flags&flagSourceHashes != 0 {
			le.PutUint32(np[g.hashSlot(c.index):], 0)
		}
	}
	newOS := len(np)
	if uint64(newOS)+offsetsSize+uint64(len(trailer)) > payloadLimit {
		return nil, nil, fmt.Errorf("payload would exceed 4 GiB")
	}
	np = append(np, g.payload[g.offsetsStart:]...)
	le.PutUint64(np[newOS:], uint64(newOS))
	if appended {
		le.PutUint32(np[newOS+28:], g.flags&^flagContiguousSources)
	}
	return np, changes, nil
}
