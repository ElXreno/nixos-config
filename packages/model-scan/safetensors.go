package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"sort"
)

var stDtype = map[string]float64{
	"F64": 8, "F32": 4, "F16": 2, "BF16": 2, "I64": 8, "I32": 4, "I16": 2, "I8": 1,
	"U64": 8, "U32": 4, "U16": 2, "U8": 1, "BOOL": 1, "C64": 8,
	"F8_E4M3": 1, "F8_E5M2": 1, "F8_E8M0": 1, "F4": 0.5, "F6_E2M3": 0.75, "F6_E3M2": 0.75,
}

type stTensor struct {
	name  string
	dtype string
	shape []uint64
	off   [2]uint64
}

func scanSafetensors(t *target, name string, ra io.ReaderAt, size int64, local bool) {
	var hb [8]byte
	if _, err := ra.ReadAt(hb[:], 0); err != nil {
		t.add(high, name, "cannot read the header length: %v", err)
		return
	}
	hl := binary.LittleEndian.Uint64(hb[:])
	if hl > 100<<20 || int64(hl)+8 > size {
		t.add(high, name, "header length %d does not fit the %d-byte file", hl, size)
		return
	}
	h := make([]byte, hl)
	if _, err := ra.ReadAt(h, 8); err != nil {
		t.add(high, name, "cannot read the header: %v", err)
		return
	}
	var hdr map[string]json.RawMessage
	if err := json.Unmarshal(h, &hdr); err != nil {
		t.add(high, name, "header is not JSON: %v", err)
		return
	}
	var ts []stTensor
	for k, v := range hdr {
		if k == "__metadata__" {
			var md map[string]string
			if err := json.Unmarshal(v, &md); err != nil {
				t.add(med, name, "__metadata__ is not a string map")
				continue
			}
			for mk, mv := range md {
				checkText(t, name+"#"+mk, mv, false)
			}
			continue
		}
		var ti struct {
			Dtype string    `json:"dtype"`
			Shape []uint64  `json:"shape"`
			Off   [2]uint64 `json:"data_offsets"`
		}
		if err := json.Unmarshal(v, &ti); err != nil {
			t.add(high, name, "tensor %q entry is malformed: %v", k, err)
			continue
		}
		ts = append(ts, stTensor{k, ti.Dtype, ti.Shape, ti.Off})
	}
	base := 8 + hl
	data := uint64(size) - base
	sort.Slice(ts, func(a, b int) bool { return ts[a].off[0] < ts[b].off[0] })
	var pos, gaps uint64
	var gapRanges [][2]uint64
	for _, x := range ts {
		if x.off[1] < x.off[0] || x.off[1] > data {
			t.add(high, name, "tensor %s range %v is outside the data section", x.name, x.off)
			continue
		}
		if w, ok := stDtype[x.dtype]; ok {
			n := 1.0
			for _, d := range x.shape {
				n *= float64(d)
			}
			if need := uint64(math.Ceil(n * w)); need != x.off[1]-x.off[0] {
				t.add(high, name, "tensor %s spans %d bytes, its %s %v needs %d", x.name, x.off[1]-x.off[0], x.dtype, x.shape, need)
			}
		}
		switch {
		case x.off[0] > pos:
			gaps += x.off[0] - pos
			gapRanges = append(gapRanges, [2]uint64{pos, x.off[0]})
		case x.off[0] < pos:
			t.add(high, name, "tensor %s overlaps the previous tensor", x.name)
		}
		pos = max(pos, x.off[1])
	}
	if gaps > 0 {
		if !local {
			t.add(med, name, "%d unreferenced bytes between tensors (the safetensors library rejects holes); content not checked remotely", gaps)
		} else {
			var nonzero int64
			var s string
			for _, g := range gapRanges {
				if nz, sm := zeroCheck(ra, int64(base+g[0]), int64(g[1]-g[0])); nz > 0 {
					nonzero += nz
					s = sm
				}
			}
			if nonzero > 0 {
				t.add(high, name, "%d unreferenced bytes between tensors hold %d non-zero bytes: %s", gaps, nonzero, s)
			} else {
				t.add(info, name, "%d zero bytes between tensors (alignment by a custom writer; the safetensors library rejects holes)", gaps)
			}
		}
	}
	if pos < data {
		tail := data - pos
		nz, s := zeroCheck(ra, int64(base+pos), int64(min(tail, 64<<20)))
		if nz > 0 {
			t.add(crit, name, "%d bytes after the last tensor: %s", tail, s)
		} else {
			t.add(med, name, "%d zero bytes after the last tensor", tail)
		}
	}
	if len(ts) == 0 {
		t.add(med, name, "no tensors in the header")
	}
}
