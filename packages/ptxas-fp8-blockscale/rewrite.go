package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Result struct {
	PTX   string
	Count int
	Skip  string
}

var (
	mmaRe     = regexp.MustCompile(`(@!?%[\w$]+\s+)?mma\.sync\.aligned\.(?:kind::f8f6f4\.)?m16n8k32\.row\.col\.f32\.(e4m3|e5m2)\.(e4m3|e5m2)\.f32\s+([^;]*);`)
	targetRe  = regexp.MustCompile(`(?m)^\s*\.target\s+sm_(\d+)([af]?)\b`)
	versionRe = regexp.MustCompile(`(?m)^\s*\.version\s+(\d+)\.(\d+)`)
)

const scaleOne = 0x7f7f7f7f

func Rewrite(src string) Result {
	if !mmaRe.MatchString(src) {
		return Result{PTX: src}
	}
	t := targetRe.FindStringSubmatch(src)
	switch {
	case t == nil:
		return Result{PTX: src, Skip: "no .target"}
	case t[1] != "120":
		return Result{PTX: src, Skip: "target sm_" + t[1] + t[2]}
	case t[2] == "":
		return Result{PTX: src, Skip: "target sm_120 lacks the block-scaled MMA, needs sm_120a or sm_120f"}
	}
	if v := versionRe.FindStringSubmatch(src); v != nil {
		major, _ := strconv.Atoi(v[1])
		minor, _ := strconv.Atoi(v[2])
		if major*10+minor < 87 {
			return Result{PTX: src, Skip: "PTX ISA " + v[1] + "." + v[2] + " predates 8.7"}
		}
	}
	n := 0
	out := mmaRe.ReplaceAllStringFunc(src, func(m string) string {
		s := mmaRe.FindStringSubmatch(m)
		n++
		return blockScaled(s[1], s[2], s[3], strings.TrimSpace(s[4]))
	})
	return Result{PTX: out, Count: n}
}

func blockScaled(pred, atype, btype, operands string) string {
	var b strings.Builder
	b.WriteString("{ .reg .b32 %fp8bs_s; .reg .b16 %fp8bs_z; ")
	fmt.Fprintf(&b, "mov.b32 %%fp8bs_s, %d; mov.b16 %%fp8bs_z, 0; ", scaleOne)
	b.WriteString(pred)
	fmt.Fprintf(&b, "mma.sync.aligned.kind::mxf8f6f4.block_scale.scale_vec::1X.m16n8k32.row.col.f32.%s.%s.f32.ue8m0 ", atype, btype)
	b.WriteString(operands)
	b.WriteString(", {%fp8bs_s}, {%fp8bs_z, %fp8bs_z}, {%fp8bs_s}, {%fp8bs_z, %fp8bs_z}; }")
	return b.String()
}
