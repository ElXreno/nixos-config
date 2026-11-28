package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var safeGlobals = setOf(
	"collections OrderedDict",
	"torch._utils _rebuild_tensor", "torch._utils _rebuild_tensor_v2", "torch._utils _rebuild_tensor_v3",
	"torch._utils _rebuild_parameter", "torch._utils _rebuild_parameter_with_state", "torch._utils _rebuild_qtensor",
	"torch._utils _rebuild_sparse_tensor", "torch._utils _rebuild_meta_tensor_no_storage",
	"torch._utils _rebuild_device_tensor_from_numpy", "torch Size", "torch device",
	"numpy.core.multiarray _reconstruct", "numpy._core.multiarray _reconstruct", "numpy ndarray", "numpy dtype",
	"numpy.core.multiarray scalar", "numpy._core.multiarray scalar", "_codecs encode",
	"builtins set", "builtins frozenset", "builtins slice", "builtins bytearray", "builtins complex",
	"__builtin__ set", "__builtin__ frozenset", "__builtin__ slice", "__builtin__ bytearray", "__builtin__ complex",
)

var torchDtypes = setOf("float16", "float32", "float64", "bfloat16", "half", "float", "double", "int8", "int16", "int32",
	"int64", "uint8", "uint16", "uint32", "uint64", "bool", "complex64", "complex128", "float8_e4m3fn", "float8_e5m2")

var dangerMods = setOf("os", "posix", "nt", "subprocess", "sys", "socket", "shutil", "runpy", "pty", "webbrowser", "requests",
	"urllib", "urllib2", "httplib", "http", "ftplib", "telnetlib", "smtplib", "pickle", "_pickle", "cPickle", "dill", "marshal",
	"importlib", "ctypes", "code", "codeop", "commands", "timeit", "pdb", "bdb", "asyncio", "multiprocessing", "threading",
	"platform", "tempfile", "glob", "pathlib", "io", "zipimport", "operator", "functools", "types", "builtins", "__builtin__",
	"pip", "setuptools", "distutils", "git", "torch.storage", "torch.serialization", "torch.hub", "torch.jit", "numpy.testing")

func safeGlobal(mod, name string) bool {
	if safeGlobals[mod+" "+name] {
		return true
	}
	return mod == "torch" && (strings.HasSuffix(name, "Storage") || torchDtypes[name])
}

func checkGlobal(t *target, file, mod, name string) {
	if safeGlobal(mod, name) {
		return
	}
	top := mod
	if i := strings.IndexByte(mod, '.'); i >= 0 {
		top = mod[:i]
	}
	if dangerMods[mod] || dangerMods[top] {
		t.add(crit, file, "pickle imports %s.%s: unpickling runs it", mod, name)
		return
	}
	t.add(high, file, "pickle imports %s.%s, outside the tensor allowlist", mod, name)
}

type pval struct {
	s     string
	isStr bool
}

func scanPickle(t *target, file string, data []byte) (end int) {
	pos := 0
	defer func() {
		if e := recover(); e != nil {
			t.add(med, file, "pickle truncated or malformed at byte %d: %v", pos, e)
			end = -1
		}
	}()
	var stack []pval
	var marks []int
	memo := map[uint64]pval{}
	other := pval{}
	need := func(n uint64) []byte {
		if n > uint64(len(data)-pos) {
			panic("operand runs past the end")
		}
		b := data[pos : pos+int(n)]
		pos += int(n)
		return b
	}
	line := func() string {
		i := bytes.IndexByte(data[pos:], '\n')
		if i < 0 {
			panic("unterminated line operand")
		}
		s := string(data[pos : pos+i])
		pos += i + 1
		return s
	}
	u8 := func() uint64 { return uint64(need(1)[0]) }
	u32 := func() uint64 { return uint64(binary.LittleEndian.Uint32(need(4))) }
	u64 := func() uint64 { return binary.LittleEndian.Uint64(need(8)) }
	push := func(v pval) { stack = append(stack, v) }
	pop := func(n int) {
		if len(stack) < n {
			panic("stack underflow")
		}
		stack = stack[:len(stack)-n]
	}
	popMark := func() {
		if len(marks) == 0 {
			panic("no mark")
		}
		m := marks[len(marks)-1]
		marks = marks[:len(marks)-1]
		if m > len(stack) {
			panic("mark above the stack")
		}
		stack = stack[:m]
	}
	top := func() pval {
		if len(stack) == 0 {
			panic("empty stack")
		}
		return stack[len(stack)-1]
	}
	str := func(b []byte) { push(pval{string(b), true}) }
	for pos < len(data) {
		op := data[pos]
		pos++
		switch op {
		case 0x80:
			need(1)
		case 0x95:
			need(8)
		case '.':
			return pos
		case 'N', 0x88, 0x89, ']', ')', '}', 0x8f, 0x97:
			push(other)
		case 0x98:
		case 'I', 'L', 'F':
			line()
			push(other)
		case 'J':
			need(4)
			push(other)
		case 'K':
			need(1)
			push(other)
		case 'M':
			need(2)
			push(other)
		case 'G':
			need(8)
			push(other)
		case 0x8a:
			need(u8())
			push(other)
		case 0x8b:
			need(u32())
			push(other)
		case 'S':
			s := line()
			if u, err := strconv.Unquote(s); err == nil {
				s = u
			} else {
				s = strings.Trim(s, `'"`)
			}
			push(pval{s, true})
		case 'T':
			str(need(u32()))
		case 'U':
			str(need(u8()))
		case 'B':
			need(u32())
			push(other)
		case 'C':
			need(u8())
			push(other)
		case 0x8e, 0x96:
			need(u64())
			push(other)
		case 'V':
			push(pval{line(), true})
		case 0x8c:
			str(need(u8()))
		case 'X':
			str(need(u32()))
		case 0x8d:
			str(need(u64()))
		case 'a', 's', '0':
			pop(map[byte]int{'a': 1, 's': 2, '0': 1}[op])
		case 'e', 'u', 0x90, '1':
			popMark()
		case 'l', 't', 'd', 0x91, 'o':
			popMark()
			push(other)
		case 0x85, 0x86, 0x87:
			pop(int(op - 0x84))
			push(other)
		case '2':
			push(top())
		case '(':
			marks = append(marks, len(stack))
		case 'g':
			i, _ := strconv.ParseUint(line(), 10, 64)
			push(memo[i])
		case 'h':
			push(memo[u8()])
		case 'j':
			push(memo[u32()])
		case 'p':
			i, _ := strconv.ParseUint(line(), 10, 64)
			memo[i] = top()
		case 'q':
			memo[u8()] = top()
		case 'r':
			memo[u32()] = top()
		case 0x94:
			memo[uint64(len(memo))] = top()
		case 0x82, 0x83, 0x84:
			need(map[byte]uint64{0x82: 1, 0x83: 2, 0x84: 4}[op])
			t.add(high, file, "pickle EXT opcode at byte %d resolves a global through the copyreg extension registry", pos-1)
			push(other)
		case 'c':
			mod := line()
			name := line()
			checkGlobal(t, file, mod, name)
			push(other)
		case 'i':
			mod := line()
			name := line()
			checkGlobal(t, file, mod, name)
			popMark()
			push(other)
		case 0x93:
			if len(stack) < 2 {
				panic("stack underflow")
			}
			name, mod := stack[len(stack)-1], stack[len(stack)-2]
			pop(2)
			if !name.isStr || !mod.isStr {
				t.add(high, file, "pickle STACK_GLOBAL at byte %d takes computed operands", pos-1)
			} else {
				checkGlobal(t, file, mod.s, name.s)
			}
			push(other)
		case 'R', 0x81:
			pop(2)
			push(other)
		case 'b':
			pop(1)
		case 0x92:
			pop(3)
			push(other)
		case 'P':
			line()
			push(other)
		case 'Q':
			pop(1)
			push(other)
		default:
			t.add(high, file, "unknown pickle opcode %#x at byte %d", op, pos-1)
			return -1
		}
	}
	t.add(med, file, "pickle ends without STOP")
	return -1
}

var legacyTorchMagic = []byte("\x80\x02\x8a\x0a\x6c\xfc\x9c\x46\xf9\x20\x6a\xa8\x50\x19.")

func scanPickleFile(t *target, name string, ra io.ReaderAt, size int64, head []byte, ext string) bool {
	legacy := bytes.HasPrefix(head, legacyTorchMagic)
	if ext == ".bin" && !legacy {
		return false
	}
	if !isPickle(head) {
		if ext == ".pkl" || ext == ".pickle" || ext == ".joblib" {
			t.add(med, name, "not a protocol 2+ pickle: not checked")
		}
		return false
	}
	data := make([]byte, min(size, 256<<20))
	n, err := ra.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		t.add(errs, name, "read: %v", err)
		return false
	}
	data = data[:n]
	limit := 16
	if legacy {
		limit = 5
	}
	for pos, i := 0, 0; i < limit && pos < len(data) && isPickle(data[pos:]); i++ {
		end := scanPickle(t, name, data[pos:])
		if end < 0 {
			return true
		}
		pos += end
	}
	return true
}

var torchZipMember = regexp.MustCompile(`(^|/)(data/\d+|version|\.data/.*|byteorder|\.format_version|\.storage_alignment)$`)

func scanZip(t *target, name string, ra io.ReaderAt, size int64) {
	z, err := zip.NewReader(ra, size)
	if err != nil {
		t.add(high, name, "ZIP container does not parse: %v", err)
		return
	}
	var code []string
	for _, f := range z.File {
		switch {
		case strings.HasSuffix(f.Name, ".pkl"):
			if f.UncompressedSize64 > 256<<20 {
				t.add(med, name, "%s: %d-byte pickle not checked", f.Name, f.UncompressedSize64)
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.add(errs, name, "%s: %v", f.Name, err)
				continue
			}
			b, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.add(errs, name, "%s: %v", f.Name, err)
				continue
			}
			scanPickle(t, name+"#"+f.Name, b)
		case strings.HasSuffix(f.Name, ".npy"):
			t.add(info, name, "%s: numpy array inside the archive", f.Name)
		case strings.Contains(f.Name, "/code/") || strings.HasSuffix(f.Name, ".py"):
			code = append(code, f.Name)
		case !torchZipMember.MatchString(f.Name):
			t.add(info, name, "unexpected archive member %s", f.Name)
		}
	}
	if len(code) > 0 {
		t.add(high, name, "TorchScript or Python code inside the archive (%d files, e.g. %s): torch.jit.load runs it", len(code), code[0])
	}
}

func scanNpy(t *target, name string, ra io.ReaderAt, size int64) {
	h := make([]byte, min(size, 1<<16))
	n, _ := ra.ReadAt(h, 0)
	h = h[:n]
	if !bytes.HasPrefix(h, []byte("\x93NUMPY")) || len(h) < 12 {
		t.add(med, name, "not a .npy file")
		return
	}
	hl, off := int(binary.LittleEndian.Uint16(h[8:])), 10
	if h[6] >= 2 {
		hl, off = int(binary.LittleEndian.Uint32(h[8:])), 12
	}
	if off+hl > len(h) {
		t.add(med, name, ".npy header of %d bytes", hl)
		return
	}
	hdr := string(h[off : off+hl])
	if strings.Contains(hdr, "'|O'") || strings.Contains(hdr, "'O'") || strings.Contains(hdr, "\"|O\"") {
		t.add(high, name, "object dtype: numpy.load(allow_pickle=True) unpickles it")
		data := make([]byte, min(size-int64(off+hl), 256<<20))
		m, _ := ra.ReadAt(data, int64(off+hl))
		scanPickle(t, name, data[:m])
	}
}
