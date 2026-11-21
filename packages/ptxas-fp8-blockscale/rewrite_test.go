package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const header = ".version 9.4\n.target sm_120a\n.address_size 64\n"

func TestRewrite(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		count int
		skip  string
		want  []string
	}{
		{
			name:  "plain e4m3",
			src:   header + "\tmma.sync.aligned.m16n8k32.row.col.f32.e4m3.e4m3.f32 {%r1,%r2,%r3,%r4}, {%r5,%r6,%r7,%r8}, {%r9,%r10}, {%r1,%r2,%r3,%r4};\n",
			count: 1,
			want: []string{
				"mma.sync.aligned.kind::mxf8f6f4.block_scale.scale_vec::1X.m16n8k32.row.col.f32.e4m3.e4m3.f32.ue8m0 {%r1,%r2,%r3,%r4}, {%r5,%r6,%r7,%r8}, {%r9,%r10}, {%r1,%r2,%r3,%r4}, {%fp8bs_s}, {%fp8bs_z, %fp8bs_z}, {%fp8bs_s}, {%fp8bs_z, %fp8bs_z};",
				"mov.b32 %fp8bs_s, 2139062143;",
			},
		},
		{
			name:  "kind f8f6f4 mixed types",
			src:   header + "mma.sync.aligned.kind::f8f6f4.m16n8k32.row.col.f32.e5m2.e4m3.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f5,%f6,%f7,%f8};\n",
			count: 1,
			want:  []string{"row.col.f32.e5m2.e4m3.f32.ue8m0 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f5,%f6,%f7,%f8}, {%fp8bs_s}"},
		},
		{
			name:  "predicate stays on the instruction",
			src:   header + "@!%p3 mma.sync.aligned.m16n8k32.row.col.f32.e4m3.e5m2.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f1,%f2,%f3,%f4};\n",
			count: 1,
			want:  []string{"mov.b16 %fp8bs_z, 0; @!%p3 mma.sync.aligned.kind::mxf8f6f4.block_scale"},
		},
		{
			name:  "family target",
			src:   ".version 9.0\n.target sm_120f\n" + "mma.sync.aligned.m16n8k32.row.col.f32.e4m3.e4m3.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f1,%f2,%f3,%f4};\n",
			count: 1,
		},
		{
			name: "f16 accumulator is already full rate",
			src:  header + "mma.sync.aligned.m16n8k32.row.col.f16.e4m3.e4m3.f16 {%r1,%r2}, {%r3,%r4,%r5,%r6}, {%r7,%r8}, {%r1,%r2};\n",
		},
		{
			name: "k16 has no block-scaled form",
			src:  header + "mma.sync.aligned.m16n8k16.row.col.f32.e4m3.e4m3.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2}, {%r3}, {%f1,%f2,%f3,%f4};\n",
		},
		{
			name: "fp4 operands are left alone",
			src:  header + "mma.sync.aligned.kind::f8f6f4.m16n8k32.row.col.f32.e2m1.e2m1.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f1,%f2,%f3,%f4};\n",
		},
		{
			name: "plain sm_120 target",
			src:  ".version 9.4\n.target sm_120\n" + "mma.sync.aligned.m16n8k32.row.col.f32.e4m3.e4m3.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f1,%f2,%f3,%f4};\n",
			skip: "target sm_120 lacks",
		},
		{
			name: "Ada",
			src:  ".version 8.8\n.target sm_89\n" + "mma.sync.aligned.m16n8k32.row.col.f32.e4m3.e4m3.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f1,%f2,%f3,%f4};\n",
			skip: "target sm_89",
		},
		{
			name: "GB10 runs FP32 accumulation at full rate",
			src:  ".version 9.0\n.target sm_121a\n" + "mma.sync.aligned.m16n8k32.row.col.f32.e4m3.e4m3.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f1,%f2,%f3,%f4};\n",
			skip: "target sm_121a",
		},
		{
			name: "old PTX ISA",
			src:  ".version 8.6\n.target sm_120a\n" + "mma.sync.aligned.m16n8k32.row.col.f32.e4m3.e4m3.f32 {%f1,%f2,%f3,%f4}, {%r1,%r2,%r3,%r4}, {%r5,%r6}, {%f1,%f2,%f3,%f4};\n",
			skip: "PTX ISA 8.6",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Rewrite(c.src)
			if res.Count != c.count {
				t.Fatalf("count %d, want %d\n%s", res.Count, c.count, res.PTX)
			}
			if !strings.HasPrefix(res.Skip, c.skip) || (c.skip == "" && res.Skip != "") {
				t.Fatalf("skip %q, want prefix %q", res.Skip, c.skip)
			}
			if c.count == 0 && res.PTX != c.src {
				t.Fatalf("untouched input changed:\n%s", res.PTX)
			}
			for _, w := range c.want {
				if !strings.Contains(res.PTX, w) {
					t.Fatalf("missing %q in\n%s", w, res.PTX)
				}
			}
		})
	}
}

func TestFixture(t *testing.T) {
	src, err := os.ReadFile("testdata/sm120a.ptx")
	if err != nil {
		t.Fatal(err)
	}
	res := Rewrite(string(src))
	if res.Count != 3 {
		t.Fatalf("rewrote %d, want 3", res.Count)
	}
	if !strings.Contains(res.PTX, "mma.sync.aligned.m16n8k32.row.col.f16.e4m3.e4m3.f16") {
		t.Fatal("f16 accumulation was rewritten")
	}
}

func TestTritonPtxas(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "site-packages", "triton", "backends", "nvidia", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	ptxas := filepath.Join(bin, "ptxas-blackwell")
	if err := os.WriteFile(ptxas, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(root, "site-packages", "triton", "_C", "libtriton.so")
	maps := "7f00-7f01 r--p 00000000 00:1e 123 /usr/lib/libc.so.6\n" +
		"7f02-7f03 r-xp 00001000 00:1e 456                        " + lib + "\n"
	if got := tritonPtxas(maps); got != ptxas {
		t.Fatalf("got %q, want %q", got, ptxas)
	}
	if got := tritonPtxas("7f00-7f01 r--p 00000000 00:1e 123 /usr/lib/libc.so.6\n"); got != "" {
		t.Fatalf("found %q without triton", got)
	}
}
