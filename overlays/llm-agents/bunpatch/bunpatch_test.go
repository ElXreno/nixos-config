package main

import (
	"bytes"
	"debug/elf"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tmod struct {
	name string
	src  []byte
	enc  byte
}

func testModules() []tmod {
	return []tmod{
		{"/$bunfs/root/cli", []byte(`import"./a.js";console.log("alpha beta gamma")`), encLatin1},
		{"/$bunfs/root/a.js", []byte(`export const a="first module";`), encUTF8},
		{"/$bunfs/root/b.js", append([]byte(`export const b="caf`), 0xe9, '"', ';'), encLatin1},
		{"/$bunfs/root/w.js", []byte("e\x00x\x00"), encUTF16},
	}
}

func buildPayload(mods []tmod, flags uint32) []byte {
	var data []byte
	add := func(b []byte) span {
		s := span{uint32(len(data)), uint32(len(b))}
		data = append(data, b...)
		return s
	}
	slab := add(bytes.Repeat([]byte{0xbc}, 64))
	type rec struct {
		name, contents, bc, info, origin span
		enc                              byte
	}
	var recs []rec
	for i, m := range mods {
		ns := add([]byte(m.name))
		data = append(data, 0)
		cs := add(m.src)
		data = append(data, 0)
		ors := add([]byte("/origin" + m.name))
		recs = append(recs, rec{ns, cs, span{slab.off + uint32(i), slab.n - uint32(i)}, span{uint32(len(data)), 0}, ors, m.enc})
	}
	table := len(data)
	for _, r := range recs {
		b := make([]byte, recSize)
		putSpan(b, 0, r.name)
		putSpan(b, 8, r.contents)
		putSpan(b, 24, r.bc)
		putSpan(b, 32, r.info)
		putSpan(b, 40, r.origin)
		b[48], b[49], b[50] = r.enc, loaderJS, 1
		data = append(data, b...)
	}
	for i := range recs {
		h := make([]byte, 4)
		le.PutUint32(h, 0xa0000000+uint32(i))
		data = append(data, h...)
	}
	argv := add([]byte("--smol"))
	os := len(data)
	off := make([]byte, offsetsSize)
	le.PutUint64(off, uint64(os))
	putSpan(off, 8, span{uint32(table), uint32(len(recs) * recSize)})
	putSpan(off, 20, argv)
	le.PutUint32(off[28:], flags)
	return append(append(data, off...), trailer...)
}

type elfOpts struct {
	rwExtra    uint64
	tailInsert bool
}

func buildELF(payload []byte, o elfOpts) []byte {
	const (
		page   = 0x1000
		base   = 0x400000
		rwOff  = 0x2000
		bunOff = 0x3000
	)
	shstr := []byte("\x00.text\x00.data\x00.bss\x00.bun\x00.comment\x00.shstrtab\x00.late\x00")
	name := func(s string) uint32 { return uint32(bytes.Index(shstr, []byte("\x00"+s+"\x00")) + 1) }
	bunSize := uint64(8 + len(payload))
	rwEnd := bunOff + alignUp(bunSize, page) + o.rwExtra
	comment := []byte("GCC: (synthetic) 1.0\x00")
	commentOff := rwEnd
	shstrOff := commentOff + uint64(len(comment))
	shoff := alignUp(shstrOff+uint64(len(shstr)), 8)
	type sh struct {
		name            string
		typ             elf.SectionType
		flags           elf.SectionFlag
		addr, off, size uint64
		align           uint64
	}
	secs := []sh{
		{},
		{".text", elf.SHT_PROGBITS, elf.SHF_ALLOC | elf.SHF_EXECINSTR, base + 0x1000, 0x1000, 0x100, 16},
		{".data", elf.SHT_PROGBITS, elf.SHF_ALLOC | elf.SHF_WRITE, base + rwOff, rwOff, 0x80, 8},
		{".bss", elf.SHT_NOBITS, elf.SHF_ALLOC | elf.SHF_WRITE, base + rwOff + 0x80, rwOff + 0x80, 0x100, 8},
		{".bun", elf.SHT_PROGBITS, elf.SHF_ALLOC | elf.SHF_WRITE, base + bunOff, bunOff, bunSize, 8},
		{".comment", elf.SHT_PROGBITS, 0, 0, commentOff, uint64(len(comment)), 1},
		{".shstrtab", elf.SHT_STRTAB, 0, 0, shstrOff, uint64(len(shstr)), 1},
	}
	if o.tailInsert {
		secs = append(secs, sh{".late", elf.SHT_PROGBITS, 0, 0, bunOff + bunSize, 4, 1})
	}
	file := make([]byte, shoff+uint64(len(secs))*shEntSize)
	copy(file, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	le.PutUint16(file[16:], uint16(elf.ET_EXEC))
	le.PutUint16(file[18:], uint16(elf.EM_X86_64))
	le.PutUint32(file[20:], 1)
	le.PutUint64(file[24:], base+0x1000)
	le.PutUint64(file[ehPhoff:], 64)
	le.PutUint64(file[ehShoff:], shoff)
	le.PutUint16(file[52:], 64)
	le.PutUint16(file[ehPhent:], phEntSize)
	le.PutUint16(file[ehPhnum:], 3)
	le.PutUint16(file[ehShent:], shEntSize)
	le.PutUint16(file[ehShnum:], uint16(len(secs)))
	le.PutUint16(file[62:], 6)
	phdr := func(i int, typ elf.ProgType, flags elf.ProgFlag, off, vaddr, size, align uint64) {
		h := 64 + i*phEntSize
		le.PutUint32(file[h:], uint32(typ))
		le.PutUint32(file[h+4:], uint32(flags))
		le.PutUint64(file[h+8:], off)
		le.PutUint64(file[h+16:], vaddr)
		le.PutUint64(file[h+24:], vaddr)
		le.PutUint64(file[h+32:], size)
		le.PutUint64(file[h+40:], size)
		le.PutUint64(file[h+48:], align)
	}
	phdr(0, elf.PT_LOAD, elf.PF_R|elf.PF_X, 0, base, 0x1100, page)
	phdr(1, elf.PT_LOAD, elf.PF_R|elf.PF_W, rwOff, base+rwOff, rwEnd-rwOff, page)
	phdr(2, elf.PT_GNU_STACK, elf.PF_R|elf.PF_W, 0, 0, 0, 0)
	copy(file[0x1000:], bytes.Repeat([]byte{0xc3}, 0x100))
	copy(file[rwOff:], bytes.Repeat([]byte{0xda}, 0x80))
	le.PutUint64(file[bunOff:], uint64(len(payload)))
	copy(file[bunOff+8:], payload)
	copy(file[commentOff:], comment)
	copy(file[shstrOff:], shstr)
	for i, s := range secs {
		if i == 0 {
			continue
		}
		h := shoff + uint64(i)*shEntSize
		le.PutUint32(file[h:], name(s.name))
		le.PutUint32(file[h+4:], uint32(s.typ))
		le.PutUint64(file[h+8:], uint64(s.flags))
		le.PutUint64(file[h+16:], s.addr)
		le.PutUint64(file[h+24:], s.off)
		le.PutUint64(file[h+32:], s.size)
		le.PutUint64(file[h+48:], s.align)
	}
	return append(file, []byte("TAIL")...)
}

func testFile(t *testing.T) []byte {
	t.Helper()
	return buildELF(buildPayload(testModules(), 0x3bff), elfOpts{})
}

func reparse(t *testing.T, file []byte) (*bunELF, *graph) {
	t.Helper()
	b, err := locateBun(file)
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	g, err := parseGraph(b.payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return b, g
}

func one(module, old, new string) []modulePatch {
	return []modulePatch{{Module: module, Replace: []replacement{{Old: old, New: new}}}}
}

func TestSyntheticParses(t *testing.T) {
	_, g := reparse(t, testFile(t))
	if len(g.names) != 4 || g.names[g.entry] != "/$bunfs/root/cli" {
		t.Fatalf("names %v entry %d", g.names, g.entry)
	}
}

func TestInPlace(t *testing.T) {
	in := testFile(t)
	out, _, err := patchFile(in, one("/$bunfs/root/cli", "alpha beta", "ab"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("file size %d -> %d", len(in), len(out))
	}
	ob, og := reparse(t, in)
	nb, ng := reparse(t, out)
	if nb.sec.Size != ob.sec.Size || ng.flags != og.flags {
		t.Fatalf("size %d flags %#x", nb.sec.Size, ng.flags)
	}
	want := `import"./a.js";console.log("ab gamma")` + strings.Repeat(" ", 8)
	if got := string(ng.contents(0)); got != want {
		t.Fatalf("contents %q", got)
	}
	r := ng.rec(0)
	if !bytes.Equal(ng.payload[r+16:r+49], make([]byte, 33)) || le.Uint32(ng.payload[ng.hashSlot(0):]) != 0 {
		t.Fatal("record not invalidated")
	}
	if !bytes.Equal(og.payload[og.rec(1):og.rec(1)+recSize], ng.payload[ng.rec(1):ng.rec(1)+recSize]) || le.Uint32(ng.payload[ng.hashSlot(1):]) != 0xa0000001 {
		t.Fatal("untouched module changed")
	}
}

func TestGrowWithinPage(t *testing.T) {
	in := testFile(t)
	out, _, err := patchFile(in, one("/$bunfs/root/a.js", "first module", "a much longer first module string"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("growth inside the page padding changed the file size %d -> %d", len(in), len(out))
	}
	ob, og := reparse(t, in)
	nb, ng := reparse(t, out)
	if nb.sec.Size <= ob.sec.Size || ng.flags != og.flags&^flagContiguousSources {
		t.Fatalf("size %d flags %#x", nb.sec.Size, ng.flags)
	}
	c := readSpan(ng.payload, ng.rec(1)+8)
	if int(c.off) != og.offsetsStart || string(ng.contents(1)) != `export const a="a much longer first module string";` || ng.payload[c.end()] != 0 {
		t.Fatalf("appended contents %+v %q", c, ng.contents(1))
	}
}

func TestGrowAcrossPages(t *testing.T) {
	in := testFile(t)
	long := strings.Repeat("x", 3*0x1000)
	out, _, err := patchFile(in, one("/$bunfs/root/a.js", "first module", long))
	if err != nil {
		t.Fatal(err)
	}
	ob, _ := reparse(t, in)
	nb, ng := reparse(t, out)
	delta := uint64(len(out) - len(in))
	if delta == 0 || delta%0x1000 != 0 || nb.shoff != ob.shoff+delta || nb.rw.Filesz != ob.rw.Filesz+delta || nb.rw.Memsz != ob.rw.Memsz+delta {
		t.Fatalf("delta %d shoff %#x filesz %#x", delta, nb.shoff, nb.rw.Filesz)
	}
	ef, err := elf.NewFile(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	cm, err := ef.Section(".comment").Data()
	if err != nil || string(cm) != "GCC: (synthetic) 1.0\x00" {
		t.Fatalf(".comment %q %v", cm, err)
	}
	if !bytes.HasSuffix(out, []byte("TAIL")) || !strings.Contains(string(ng.contents(1)), long) {
		t.Fatal("tail or contents lost")
	}
}

func TestLatin1Transcoded(t *testing.T) {
	out, _, err := patchFile(testFile(t), one("/$bunfs/root/b.js", "const b", "const bee"))
	if err != nil {
		t.Fatal(err)
	}
	_, ng := reparse(t, out)
	if got := string(ng.contents(2)); got != `export const bee="café";` || ng.payload[ng.rec(2)+48] != encUTF8 {
		t.Fatalf("contents %q enc %d", got, ng.payload[ng.rec(2)+48])
	}
}

func TestSeveralModulesAndSets(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`[{"module":"/$bunfs/root/cli","replace":[{"old":"alpha","new":"ALPHA"}]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`[{"module":"/$bunfs/root/a.js","replace":[{"old":"first","new":"the very first"}]},{"module":"/$bunfs/root/cli","replace":[{"old":"ALPHA beta","new":"ALPHA BETA"}]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := loadPatches([]string{a, b})
	if err != nil || len(ps) != 2 || len(ps[0].Replace) != 2 {
		t.Fatalf("patches %+v %v", ps, err)
	}
	exe := filepath.Join(dir, "exe")
	if err := os.WriteFile(exe, testFile(t), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(exe, []string{a, b}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(exe); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v %v", info.Mode(), err)
	}
	_, ng := reparse(t, out)
	if !strings.Contains(string(ng.contents(0)), `"ALPHA BETA gamma"`) || string(ng.contents(1)) != `export const a="the very first module";` {
		t.Fatalf("contents %q %q", ng.contents(0), ng.contents(1))
	}
}

func TestNoSourceHashes(t *testing.T) {
	in := buildELF(buildPayload(testModules(), 0x3bff&^flagSourceHashes), elfOpts{})
	ps := append(one("/$bunfs/root/cli", "alpha beta", "ab"), one("/$bunfs/root/a.js", "first module", "a much longer first module string")...)
	out, _, err := patchFile(in, ps)
	if err != nil {
		t.Fatal(err)
	}
	_, og := reparse(t, in)
	_, ng := reparse(t, out)
	if ng.flags != og.flags&^flagContiguousSources {
		t.Fatalf("flags %#x", ng.flags)
	}
	for i := range og.names {
		if got := le.Uint32(ng.payload[ng.hashSlot(i):]); got != 0xa0000000+uint32(i) {
			t.Fatalf("word %d after the module table changed to %#x without the source hash flag", i, got)
		}
	}
	for _, i := range []int{0, 1} {
		r := ng.rec(i)
		if !bytes.Equal(ng.payload[r+16:r+49], make([]byte, 33)) {
			t.Fatalf("record %d not invalidated", i)
		}
	}
}

func TestPayloadLimit(t *testing.T) {
	in := testFile(t)
	_, g := reparse(t, in)
	size := uint64(len(g.payload))
	appended := uint64(g.offsetsStart) + uint64(len(`export const a="a much longer first module string";`)) + 1
	defer func(v uint64) { payloadLimit = v }(payloadLimit)
	shrink := one("/$bunfs/root/cli", "alpha beta", "ab")
	grow := one("/$bunfs/root/a.js", "first module", "a much longer first module string")
	cases := []struct {
		name    string
		limit   uint64
		patches []modulePatch
		want    string
	}{
		{"in place at the limit", size, shrink, ""},
		{"in place over the limit", size - 1, shrink, "payload would exceed 4 GiB"},
		{"append over the limit", size, grow, "/$bunfs/root/a.js: payload would exceed 4 GiB"},
		{"append fits, offsets do not", appended, grow, "payload would exceed 4 GiB"},
		{"append at the limit", appended + offsetsSize + uint64(len(trailer)), grow, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			payloadLimit = c.limit
			_, _, err := patchFile(in, c.patches)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != c.want {
				t.Fatalf("err %q, want %q", got, c.want)
			}
		})
	}
}

func TestRefusals(t *testing.T) {
	base := buildPayload(testModules(), 0x3bff)
	bg, err := parseGraph(base)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		file    func() []byte
		patches []modulePatch
		want    string
	}{
		{"no match", func() []byte { return buildELF(base, elfOpts{}) }, one("/$bunfs/root/cli", "omega", "x"), "matches 0 times"},
		{"two matches", func() []byte { return buildELF(base, elfOpts{}) }, one("/$bunfs/root/cli", "a", "x"), "matches"},
		{"unknown module", func() []byte { return buildELF(base, elfOpts{}) }, one("/$bunfs/root/zz.js", "a", "x"), "not in the graph"},
		{"utf16", func() []byte { return buildELF(base, elfOpts{}) }, one("/$bunfs/root/w.js", "e", "x"), "UTF-16"},
		{"nul in new", func() []byte { return buildELF(base, elfOpts{}) }, one("/$bunfs/root/cli", "alpha", "a\x00"), "NUL-free"},
		{"empty old", func() []byte { return buildELF(base, elfOpts{}) }, one("/$bunfs/root/cli", "", "x"), "empty old"},
		{"not tail", func() []byte { return buildELF(base, elfOpts{rwExtra: 0x1000}) }, one("/$bunfs/root/cli", "alpha", "x"), "tail of its segment"},
		{"section in padding", func() []byte { return buildELF(base, elfOpts{tailInsert: true}) }, one("/$bunfs/root/cli", "alpha", "x"), "overlaps .bun"},
		{"trailer", func() []byte {
			p := append([]byte(nil), base...)
			p[len(p)-2] = 'X'
			return buildELF(p, elfOpts{})
		}, one("/$bunfs/root/cli", "alpha", "x"), "trailer"},
		{"byteCount", func() []byte {
			p := append([]byte(nil), base...)
			p[len(p)-len(trailer)-offsetsSize]++
			return buildELF(p, elfOpts{})
		}, one("/$bunfs/root/cli", "alpha", "x"), "byteCount"},
		{"section size", func() []byte {
			f := buildELF(base, elfOpts{})
			le.PutUint64(f[0x3000:], uint64(len(base)-1))
			return f
		}, one("/$bunfs/root/cli", "alpha", "x"), "u64 payload length"},
		{"loader", func() []byte {
			p := append([]byte(nil), base...)
			p[bg.rec(0)+49] = 2
			return buildELF(p, elfOpts{})
		}, one("/$bunfs/root/cli", "alpha", "x"), "loader 2 is not JS"},
		{"duplicate name", func() []byte {
			ms := testModules()
			ms[1].name = ms[0].name
			return buildELF(buildPayload(ms, 0x3bff), elfOpts{})
		}, one("/$bunfs/root/cli", "alpha", "x"), "listed twice"},
		{"record layout", func() []byte {
			ms := testModules()
			ms[1].name = "a.js"
			return buildELF(buildPayload(ms, 0x3bff), elfOpts{})
		}, one("/$bunfs/root/cli", "alpha", "x"), "record layout mismatch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := patchFile(c.file(), c.patches)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err %v, want %q", err, c.want)
			}
		})
	}
}
