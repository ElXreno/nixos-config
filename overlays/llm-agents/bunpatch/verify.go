package main

import (
	"bytes"
	"cmp"
	"debug/elf"
	"fmt"
	"slices"
)

func verify(old *bunELF, og *graph, out []byte, delta int64, changes []change) error {
	nb, err := locateBun(out)
	if err != nil {
		return fmt.Errorf("result does not parse: %v", err)
	}
	ng, err := parseGraph(nb.payload)
	if err != nil {
		return fmt.Errorf("result graph does not parse: %v", err)
	}
	d := uint64(delta)
	if nb.sec.Offset != old.sec.Offset || nb.sec.Addr != old.sec.Addr || nb.rwIndex != old.rwIndex || nb.secIndex != old.secIndex {
		return fmt.Errorf(".bun moved")
	}
	if nb.rw.Filesz != old.rw.Filesz+d || nb.rw.Memsz != old.rw.Memsz+d || nb.shoff != old.shoff+d {
		return fmt.Errorf("segment or section table not shifted by %d", delta)
	}
	head := append([]byte(nil), old.file[:old.sec.Offset]...)
	le.PutUint64(head[ehShoff:], old.shoff+d)
	ph := old.phoff + uint64(old.rwIndex)*phEntSize
	le.PutUint64(head[ph+32:], old.rw.Filesz+d)
	le.PutUint64(head[ph+40:], old.rw.Memsz+d)
	if !bytes.Equal(head, out[:old.sec.Offset]) {
		return fmt.Errorf("bytes before .bun changed beyond the expected header fields")
	}
	oe, err := elf.NewFile(bytes.NewReader(old.file))
	if err != nil {
		return err
	}
	ne, err := elf.NewFile(bytes.NewReader(out))
	if err != nil {
		return err
	}
	if len(oe.Sections) != len(ne.Sections) || len(oe.Progs) != len(ne.Progs) {
		return fmt.Errorf("header counts changed")
	}
	for i, os := range oe.Sections {
		ns := ne.Sections[i]
		want := os.SectionHeader
		if i == old.secIndex {
			want.Size = nb.sec.Size
			want.FileSize = nb.sec.Size
		} else if os.Type != elf.SHT_NOBITS && os.Offset >= old.rwEnd {
			want.Offset += d
			if !bytes.Equal(old.file[os.Offset:os.Offset+os.FileSize], out[want.Offset:want.Offset+os.FileSize]) {
				return fmt.Errorf("section %s contents changed", os.Name)
			}
		}
		if ns.SectionHeader != want {
			return fmt.Errorf("section %s header %+v, want %+v", os.Name, ns.SectionHeader, want)
		}
	}
	if !bytes.Equal(old.file[old.shoff+uint64(old.shnum)*shEntSize:], out[nb.shoff+uint64(nb.shnum)*shEntSize:]) {
		return fmt.Errorf("bytes after the section header table changed")
	}
	if ng.modules != og.modules || ng.entry != og.entry || ng.argv != og.argv || len(ng.names) != len(og.names) {
		return fmt.Errorf("offsets block changed")
	}
	wantFlags := og.flags
	var allowed [][2]uint64
	var tail []byte
	hashed := og.flags&flagSourceHashes != 0
	for _, c := range changes {
		i := c.index
		or, nr := og.payload[og.rec(i):og.rec(i)+recSize], ng.payload[ng.rec(i):ng.rec(i)+recSize]
		if !bytes.Equal(nr[16:49], make([]byte, 33)) || !bytes.Equal(or[:8], nr[:8]) || !bytes.Equal(or[49:], nr[49:]) {
			return fmt.Errorf("%s: record not invalidated as expected", c.module)
		}
		if hashed && le.Uint32(ng.payload[ng.hashSlot(i):]) != 0 {
			return fmt.Errorf("%s: source hash slot not cleared", c.module)
		}
		oc, nc := readSpan(or, 8), readSpan(nr, 8)
		if c.appended {
			wantFlags &^= flagContiguousSources
			if uint64(nc.off) != uint64(og.offsetsStart)+uint64(len(tail)) || int(nc.n) != len(c.source) {
				return fmt.Errorf("%s: appended contents at %+v, want offset %d", c.module, nc, og.offsetsStart+len(tail))
			}
			tail = append(append(tail, c.source...), 0)
		} else {
			got := ng.contents(i)
			if nc != oc || !bytes.Equal(got[:len(c.source)], c.source) || len(bytes.Trim(got[len(c.source):], " ")) != 0 {
				return fmt.Errorf("%s: in-place contents differ from the patched source", c.module)
			}
			allowed = append(allowed, [2]uint64{uint64(oc.off), oc.end()})
		}
		allowed = append(allowed, [2]uint64{uint64(og.rec(i) + 8), uint64(og.rec(i) + 49)})
		if hashed {
			allowed = append(allowed, [2]uint64{uint64(og.hashSlot(i)), uint64(og.hashSlot(i) + 4)})
		}
	}
	if ng.offsetsStart != og.offsetsStart+len(tail) || !bytes.Equal(ng.payload[og.offsetsStart:ng.offsetsStart], tail) {
		return fmt.Errorf("appended region differs from the appended sources")
	}
	slices.SortFunc(allowed, func(a, b [2]uint64) int { return cmp.Compare(a[0], b[0]) })
	at := uint64(0)
	for _, r := range append(allowed, [2]uint64{uint64(og.offsetsStart), uint64(og.offsetsStart)}) {
		if r[0] < at {
			return fmt.Errorf("patched regions overlap at payload offset %d", r[0])
		}
		if !bytes.Equal(og.payload[at:r[0]], ng.payload[at:r[0]]) {
			return fmt.Errorf("payload bytes %d..%d changed outside the patched modules", at, r[0])
		}
		at = r[1]
	}
	wantOffsets := append([]byte(nil), og.payload[og.offsetsStart:]...)
	le.PutUint64(wantOffsets, uint64(ng.offsetsStart))
	le.PutUint32(wantOffsets[28:], wantFlags)
	if !bytes.Equal(wantOffsets, ng.payload[ng.offsetsStart:]) {
		return fmt.Errorf("offsets block or trailer differ from the expected update")
	}
	return nil
}
