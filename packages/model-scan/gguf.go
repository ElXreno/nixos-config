package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

var ggmlBlocks = map[uint32][2]uint64{
	0: {1, 4}, 1: {1, 2}, 2: {32, 18}, 3: {32, 20}, 6: {32, 22}, 7: {32, 24}, 8: {32, 34}, 9: {32, 36},
	10: {256, 84}, 11: {256, 110}, 12: {256, 144}, 13: {256, 176}, 14: {256, 210}, 15: {256, 292},
	16: {256, 66}, 17: {256, 74}, 18: {256, 98}, 19: {256, 50}, 20: {32, 18}, 21: {256, 110},
	22: {256, 82}, 23: {256, 136}, 24: {1, 1}, 25: {1, 2}, 26: {1, 4}, 27: {1, 8}, 28: {1, 8},
	29: {256, 56}, 30: {1, 2}, 34: {256, 54}, 35: {256, 66}, 39: {32, 17},
}

var ggufScalar = map[uint32]uint64{0: 1, 1: 1, 2: 2, 3: 2, 4: 4, 5: 4, 6: 4, 7: 1, 10: 8, 11: 8, 12: 8}

type rd struct {
	r    *bufio.Reader
	n    int64
	size int64
}

func (r *rd) read(b []byte) {
	if _, err := io.ReadFull(r.r, b); err != nil {
		panic(fmt.Errorf("header truncated at byte %d: %w", r.n, err))
	}
	r.n += int64(len(b))
}

func (r *rd) u32() uint32 {
	var b [4]byte
	r.read(b[:])
	return binary.LittleEndian.Uint32(b[:])
}

func (r *rd) u64() uint64 {
	var b [8]byte
	r.read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

func (r *rd) left() uint64 { return uint64(r.size - r.n) }

func (r *rd) skip(n uint64) {
	if n > r.left() {
		panic(fmt.Errorf("value of %d bytes at byte %d runs past the end of the file", n, r.n))
	}
	if _, err := r.r.Discard(int(n)); err != nil {
		panic(err)
	}
	r.n += int64(n)
}

func (r *rd) str() string {
	l := r.u64()
	if l > r.left() {
		panic(fmt.Errorf("string of %d bytes at byte %d runs past the end of the file", l, r.n))
	}
	b := make([]byte, l)
	r.read(b)
	return string(b)
}

type ggufKV struct {
	key  string
	strs []string
}

func (r *rd) value(t uint32, kv *ggufKV, depth int) {
	if sz, ok := ggufScalar[t]; ok {
		r.skip(sz)
		return
	}
	switch t {
	case 8:
		kv.strs = append(kv.strs, r.str())
	case 9:
		if depth > 4 {
			panic("arrays nested deeper than 4 levels")
		}
		et := r.u32()
		n := r.u64()
		if sz, ok := ggufScalar[et]; ok {
			if n > r.left()/sz {
				panic(fmt.Errorf("array of %d elements runs past the end of the file", n))
			}
			r.skip(n * sz)
			return
		}
		if n > r.left() {
			panic(fmt.Errorf("array of %d elements runs past the end of the file", n))
		}
		for range n {
			r.value(et, kv, depth+1)
		}
	default:
		panic(fmt.Errorf("unknown metadata value type %d", t))
	}
}

type ggufTensor struct {
	name  string
	dims  []uint64
	typ   uint32
	off   uint64
	size  uint64
	known bool
}

func (x ggufTensor) elems() uint64 {
	n := uint64(1)
	for _, d := range x.dims {
		n *= d
	}
	return n
}

func alignUp(v, a uint64) uint64 { return (v + a - 1) / a * a }

func scanGGUF(t *target, name string, ra io.ReaderAt, size int64, o opts, local bool) {
	r := &rd{r: bufio.NewReaderSize(io.NewSectionReader(ra, 0, size), 1<<20), size: size}
	if m := r.u32(); m != 0x46554747 {
		t.add(high, name, "not a GGUF file (magic %#x)", m)
		return
	}
	if v := r.u32(); v < 2 || v > 3 {
		t.add(high, name, "GGUF version %d is not supported", v)
		return
	}
	nt, nkv := r.u64(), r.u64()
	if nt > uint64(size)/24 || nkv > uint64(size)/12 {
		t.add(high, name, "implausible counts: %d tensors, %d metadata keys", nt, nkv)
		return
	}
	align := uint64(32)
	seen := map[string]bool{}
	var kvs []ggufKV
	for range nkv {
		kv := ggufKV{key: r.str()}
		typ := r.u32()
		if seen[kv.key] {
			t.add(high, name, "duplicate metadata key %q: loaders disagree on which value wins", kv.key)
		}
		seen[kv.key] = true
		if kv.key == "general.alignment" && typ == 4 {
			align = uint64(r.u32())
			if align == 0 || align&(align-1) != 0 {
				t.add(high, name, "general.alignment %d is not a power of two", align)
				return
			}
			continue
		}
		r.value(typ, &kv, 0)
		kvs = append(kvs, kv)
	}
	ts := make([]ggufTensor, nt)
	for i := range ts {
		ts[i].name = r.str()
		nd := r.u32()
		if nd > 8 {
			t.add(high, name, "tensor %q has %d dimensions", ts[i].name, nd)
			return
		}
		ts[i].dims = make([]uint64, nd)
		for j := range ts[i].dims {
			ts[i].dims[j] = r.u64()
		}
		ts[i].typ = r.u32()
		ts[i].off = r.u64()
	}
	hdrEnd := uint64(r.n)
	dataStart := alignUp(hdrEnd, align)
	if dataStart > uint64(size) {
		t.add(high, name, "file ends inside the header padding")
		return
	}
	if nz, s := zeroCheck(ra, int64(hdrEnd), int64(dataStart-hdrEnd)); nz > 0 {
		t.add(high, name, "padding after the header holds %d non-zero bytes: %s", nz, s)
	}
	ggufMetadata(t, name, kvs)
	ggufTensors(t, name, ra, uint64(size), ts, dataStart, align, o, local)
}

func ggufMetadata(t *target, name string, kvs []ggufKV) {
	for _, kv := range kvs {
		switch {
		case kv.key == "tokenizer.chat_template" || strings.HasPrefix(kv.key, "tokenizer.chat_template."):
			for _, s := range kv.strs {
				t.addTemplate(name+"#"+kv.key, s)
			}
		case strings.HasPrefix(kv.key, "tokenizer.ggml."):
		case strings.HasPrefix(kv.key, "general.base_model.") && strings.HasSuffix(kv.key, ".repo_url"):
			for _, s := range kv.strs {
				if repo := hfRepoFromURL(s); repo != "" {
					t.addBase(repo)
				}
			}
		default:
			for _, s := range kv.strs {
				f := name + "#" + kv.key
				checkText(t, f, s, false)
				if len(s) > 1<<20 {
					t.add(med, f, "string metadata of %d bytes", len(s))
				}
				if !utf8.ValidString(s) || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' && r != '\r' }) {
					t.add(med, f, "binary data in string metadata: %s", analyzeBlob([]byte(s)))
				}
			}
		}
	}
}

type bpeEst struct {
	bpe, err float64
	elems    uint64
}

func ggufTensors(t *target, name string, ra io.ReaderAt, size uint64, ts []ggufTensor, dataStart, align uint64, o opts, local bool) {
	sort.Slice(ts, func(a, b int) bool { return ts[a].off < ts[b].off })
	unknown := map[uint32]int{}
	for i := range ts {
		x := &ts[i]
		if b, ok := ggmlBlocks[x.typ]; ok {
			e := x.elems()
			if e%b[0] != 0 {
				t.add(high, name, "tensor %s: %d elements is not a multiple of the %d-element block", x.name, e, b[0])
			}
			x.size, x.known = e/b[0]*b[1], true
		} else {
			unknown[x.typ]++
		}
		if x.off%align != 0 {
			t.add(med, name, "tensor %s offset %d is not aligned to %d", x.name, x.off, align)
		}
	}
	est := map[uint32]bpeEst{}
	var padBytes, padNonzero int64
	var padSamples []string
	for i := 0; i+1 < len(ts); i++ {
		x, next := ts[i], ts[i+1]
		if !x.known {
			if e := x.elems(); e > est[x.typ].elems {
				est[x.typ] = bpeEst{float64(next.off-x.off) / float64(e), float64(align) / float64(e), e}
			}
			continue
		}
		end := x.off + x.size
		if end > next.off {
			t.add(high, name, "tensor %s overlaps tensor %s", x.name, next.name)
			continue
		}
		gap := next.off - end
		if gap >= align {
			t.add(med, name, "%d unreferenced bytes after tensor %s", gap, x.name)
		}
		if local && gap > 0 {
			padBytes += int64(gap)
			if nz, s := zeroCheck(ra, int64(dataStart+end), int64(gap)); nz > 0 {
				padNonzero += nz
				if len(padSamples) < 3 {
					padSamples = append(padSamples, fmt.Sprintf("after %s: %s", x.name, s))
				}
			}
		}
	}
	if padNonzero > 0 {
		t.add(high, name, "padding between tensors holds %d non-zero bytes:\n%s", padNonzero, strings.Join(padSamples, "\n"))
	}
	if len(unknown) > 0 {
		t.add(info, name, "ggml types without a known block size %v: their padding is not checked", unknown)
	}
	if !local {
		t.add(info, name, "remote: padding between tensors and tensor data not checked")
	}
	ggufTail(t, name, ra, size, ts, dataStart, align, est)
	if local && o.deep {
		deadBlocks(t, name, ra, ts, dataStart)
	}
}

func ggufTail(t *target, name string, ra io.ReaderAt, size uint64, ts []ggufTensor, dataStart, align uint64, est map[uint32]bpeEst) {
	expected, tol := dataStart, uint64(0)
	if len(ts) > 0 {
		last := ts[len(ts)-1]
		start := dataStart + last.off
		switch e, ok := est[last.typ]; {
		case last.known:
			expected = start + last.size
		case ok:
			n := float64(last.elems())
			expected = start + uint64(n*e.bpe)
			tol = uint64(math.Ceil(n*e.err)) + align
		default:
			t.add(info, name, "last tensor %s has ggml type %d of unknown size: trailing data not checked", last.name, last.typ)
			return
		}
	}
	if expected > size+tol {
		t.add(high, name, "file is %d bytes shorter than its tensors need", expected-size)
		return
	}
	if expected >= size {
		return
	}
	tail := size - expected
	pad := alignUp(expected, align) - expected + tol
	if tail <= pad {
		if tol == 0 {
			if nz, s := zeroCheck(ra, int64(expected), int64(tail)); nz > 0 {
				t.add(high, name, "alignment padding after the last tensor holds %d non-zero bytes: %s", nz, s)
			}
		}
		return
	}
	nz, s := zeroCheck(ra, int64(expected), int64(min(tail, 64<<20)))
	if nz > 0 {
		t.add(crit, name, "%d bytes after the last tensor, no loader reads them: %s", tail, s)
	} else {
		t.add(med, name, "%d zero bytes after the last tensor", tail)
	}
}

func q8Underflow(qs []byte) bool {
	peak := 0
	for _, b := range qs {
		v := int(int8(b))
		if v == -128 {
			return false
		}
		peak = max(peak, v, -v)
	}
	return peak == 127
}

func q4kUnderflow(sc []byte) bool {
	for j := range 8 {
		var d, m byte
		if j < 4 {
			d, m = sc[j]&63, sc[j+4]&63
		} else {
			d, m = sc[j+4]&0xf|(sc[j-4]>>6)<<4, sc[j+4]>>4|(sc[j]>>6)<<4
		}
		if d == 63 || m == 63 {
			return true
		}
	}
	return false
}

func deadBlocks(t *target, name string, ra io.ReaderAt, ts []ggufTensor, dataStart uint64) {
	var scanned, payload, negZero, underflow int
	var hits []string
	for _, x := range ts {
		if x.typ != 8 && x.typ != 12 {
			continue
		}
		bs := uint64(34)
		if x.typ == 12 {
			bs = 144
		}
		buf := make([]byte, bs*32768)
		var p, nz int
		for done := uint64(0); done < x.size; {
			c := min(uint64(len(buf)), x.size-done)
			if _, err := ra.ReadAt(buf[:c], int64(dataStart+x.off+done)); err != nil {
				t.add(errs, name, "read tensor %s: %v", x.name, err)
				return
			}
			for b := uint64(0); b+bs <= c; b += bs {
				blk := buf[b : b+bs]
				d := binary.LittleEndian.Uint16(blk)
				if x.typ == 8 {
					if d == 0x8000 {
						nz++
					}
					if d&0x7fff == 0 && !allZero(blk[2:]) {
						if q8Underflow(blk[2:]) {
							underflow++
						} else {
							p++
						}
					}
					continue
				}
				dm := binary.LittleEndian.Uint16(blk[2:])
				if d == 0x8000 || dm == 0x8000 {
					nz++
				}
				if d&0x7fff == 0 && dm&0x7fff == 0 && !allZero(blk[4:]) {
					if q4kUnderflow(blk[4:16]) {
						underflow++
					} else {
						p++
					}
				}
			}
			done += c
		}
		scanned++
		payload += p
		negZero += nz
		if (p > 0 || nz > 0) && len(hits) < 5 {
			hits = append(hits, fmt.Sprintf("%s: %d zero-scale blocks with data, %d negative-zero scales", x.name, p, nz))
		}
	}
	if payload > 0 {
		t.add(high, name, "%d zero-scale quant blocks carry non-zero bytes that dequantize to nothing (hidden payload):\n%s", payload, strings.Join(hits, "\n"))
	} else if negZero > 0 {
		t.add(med, name, "%d quant blocks have a negative-zero scale, which reference quantizers never write:\n%s", negZero, strings.Join(hits, "\n"))
	}
	if scanned > 0 {
		t.add(info, name, "zero-scale block scan: %d Q8_0/Q4_K tensors, %d blocks whose scale underflowed to zero in FP16 (quantizer output with a full-range quant, not a payload)", scanned, underflow)
	}
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

func zeroCheck(ra io.ReaderAt, off, n int64) (int64, string) {
	const sampleMax = 1 << 20
	if n <= 0 {
		return 0, ""
	}
	var nonzero int64
	var sample []byte
	buf := make([]byte, min(n, 4<<20))
	for pos := int64(0); pos < n; {
		c := min(int64(len(buf)), n-pos)
		if _, err := ra.ReadAt(buf[:c], off+pos); err != nil && err != io.EOF {
			return 1, fmt.Sprintf("read error: %v", err)
		}
		for _, b := range buf[:c] {
			if b != 0 {
				nonzero++
			}
		}
		if len(sample) < sampleMax {
			sample = append(sample, buf[:min(c, int64(sampleMax-len(sample)))]...)
		}
		pos += c
	}
	if nonzero == 0 {
		return 0, ""
	}
	return nonzero, analyzeBlob(sample)
}
