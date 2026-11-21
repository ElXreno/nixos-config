package main

import (
	"bytes"
	"debug/elf"
	"fmt"
)

const (
	ehShoff   = 40
	ehPhoff   = 32
	ehPhent   = 54
	ehPhnum   = 56
	ehShent   = 58
	ehShnum   = 60
	phEntSize = 56
	shEntSize = 64
	minPage   = 0x1000
)

type bunELF struct {
	file     []byte
	secIndex int
	sec      elf.SectionHeader
	rwIndex  int
	rw       elf.ProgHeader
	page     uint64
	rwEnd    uint64
	shoff    uint64
	shnum    int
	phoff    uint64
	payload  []byte
}

func alignUp(v, a uint64) uint64 { return (v + a - 1) / a * a }

func locateBun(file []byte) (*bunELF, error) {
	ef, err := elf.NewFile(bytes.NewReader(file))
	if err != nil {
		return nil, err
	}
	if ef.Class != elf.ELFCLASS64 || ef.Data != elf.ELFDATA2LSB {
		return nil, fmt.Errorf("not a little-endian ELF64 file")
	}
	b := &bunELF{
		file:  file,
		shoff: le.Uint64(file[ehShoff:]),
		phoff: le.Uint64(file[ehPhoff:]),
		shnum: int(le.Uint16(file[ehShnum:])),
	}
	if le.Uint16(file[ehPhent:]) != phEntSize || le.Uint16(file[ehShent:]) != shEntSize {
		return nil, fmt.Errorf("unexpected program or section header entry size")
	}
	if b.shnum == 0 || b.shnum != len(ef.Sections) || int(le.Uint16(file[ehPhnum:])) != len(ef.Progs) {
		return nil, fmt.Errorf("extended section numbering or header count mismatch")
	}
	b.secIndex = -1
	for i, s := range ef.Sections {
		if s.Name != ".bun" {
			continue
		}
		if b.secIndex >= 0 {
			return nil, fmt.Errorf("more than one .bun section")
		}
		b.secIndex = i
		b.sec = s.SectionHeader
	}
	if b.secIndex < 0 {
		return nil, fmt.Errorf("no .bun section (overlay-format executables are not supported)")
	}
	sec := b.sec
	if sec.Type != elf.SHT_PROGBITS || sec.Flags&(elf.SHF_ALLOC|elf.SHF_WRITE) != elf.SHF_ALLOC|elf.SHF_WRITE {
		return nil, fmt.Errorf(".bun is %v %v, want SHT_PROGBITS with WRITE+ALLOC", sec.Type, sec.Flags)
	}
	if sec.Size < 8 || sec.Offset+sec.Size > uint64(len(file)) {
		return nil, fmt.Errorf(".bun section out of the file")
	}
	payloadLen := le.Uint64(file[sec.Offset:])
	if sec.Size != 8+payloadLen {
		return nil, fmt.Errorf(".bun size %d is not 8 + u64 payload length %d", sec.Size, payloadLen)
	}
	b.payload = file[sec.Offset+8 : sec.Offset+sec.Size]
	b.rwIndex = -1
	for i, p := range ef.Progs {
		if p.Type == elf.PT_LOAD && p.Off <= sec.Offset && sec.Offset < p.Off+p.Filesz {
			if b.rwIndex >= 0 {
				return nil, fmt.Errorf("two PT_LOAD segments hold .bun")
			}
			b.rwIndex = i
			b.rw = p.ProgHeader
		}
	}
	if b.rwIndex < 0 {
		return nil, fmt.Errorf("no PT_LOAD segment holds .bun")
	}
	rw := b.rw
	if rw.Flags&elf.PF_W == 0 || sec.Addr-rw.Vaddr != sec.Offset-rw.Off {
		return nil, fmt.Errorf(".bun is not in a writable segment at a matching address")
	}
	b.page = rw.Align
	if b.page < minPage || b.page&(b.page-1) != 0 {
		return nil, fmt.Errorf("segment alignment %#x is not a page size", b.page)
	}
	alloc := alignUp(sec.Size, b.page)
	b.rwEnd = rw.Off + rw.Filesz
	if sec.Offset+alloc != b.rwEnd || sec.Addr+alloc != rw.Vaddr+rw.Memsz {
		return nil, fmt.Errorf(".bun is not the page-aligned tail of its segment (file end %#x vs %#x, memory end %#x vs %#x)",
			sec.Offset+alloc, b.rwEnd, sec.Addr+alloc, rw.Vaddr+rw.Memsz)
	}
	for i, p := range ef.Progs {
		if i == b.rwIndex {
			continue
		}
		if p.Type == elf.PT_LOAD && (p.Off+p.Filesz > rw.Off || p.Vaddr+p.Memsz > rw.Vaddr) {
			return nil, fmt.Errorf("PT_LOAD %d lies after the .bun segment", i)
		}
		if p.Filesz > 0 && p.Off+p.Filesz > sec.Offset {
			return nil, fmt.Errorf("segment %d (%v) reaches into or past .bun", i, p.Type)
		}
	}
	if b.shoff < b.rwEnd || b.shoff+uint64(b.shnum)*shEntSize > uint64(len(file)) {
		return nil, fmt.Errorf("section header table at %#x is not in the file tail", b.shoff)
	}
	for i, s := range ef.Sections {
		if i == b.secIndex {
			continue
		}
		if s.Flags&elf.SHF_ALLOC != 0 && s.Size > 0 && s.Addr+s.Size > sec.Addr {
			return nil, fmt.Errorf("allocated section %s ends after .bun starts", s.Name)
		}
		if s.Type != elf.SHT_NOBITS && s.Size > 0 && s.Offset < b.rwEnd && s.Offset+s.Size > sec.Offset {
			return nil, fmt.Errorf("section %s overlaps .bun or straddles the segment end", s.Name)
		}
	}
	return b, nil
}

func (b *bunELF) rewrite(payload []byte) ([]byte, int64, error) {
	newSize := 8 + uint64(len(payload))
	if newSize < b.sec.Size {
		return nil, 0, fmt.Errorf("payload shrank from %d to %d bytes", b.sec.Size-8, len(payload))
	}
	delta := alignUp(newSize, b.page) - alignUp(b.sec.Size, b.page)
	res := make([]byte, uint64(len(b.file))+delta)
	copy(res, b.file[:b.sec.Offset])
	le.PutUint64(res[b.sec.Offset:], uint64(len(payload)))
	copy(res[b.sec.Offset+8:], payload)
	copy(res[b.rwEnd+delta:], b.file[b.rwEnd:])
	newShoff := b.shoff + delta
	le.PutUint64(res[ehShoff:], newShoff)
	for k := 0; k < b.shnum; k++ {
		h := newShoff + uint64(k)*shEntSize
		if k == b.secIndex {
			le.PutUint64(res[h+32:], newSize)
			continue
		}
		if elf.SectionType(le.Uint32(res[h+4:])) == elf.SHT_NOBITS {
			continue
		}
		if off := le.Uint64(res[h+24:]); off >= b.rwEnd {
			le.PutUint64(res[h+24:], off+delta)
		}
	}
	ph := b.phoff + uint64(b.rwIndex)*phEntSize
	le.PutUint64(res[ph+32:], b.rw.Filesz+delta)
	le.PutUint64(res[ph+40:], b.rw.Memsz+delta)
	return res, int64(delta), nil
}
