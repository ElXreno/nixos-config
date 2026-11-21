package treesitter

// #include "dl.h"
import "C"

import (
	"errors"
	"strings"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func dlopen(parser, name string) (*sitter.Language, error) {
	cpath := C.CString(parser)
	defer C.free(unsafe.Pointer(cpath))
	csym := C.CString("tree_sitter_" + strings.ReplaceAll(name, "-", "_"))
	defer C.free(unsafe.Pointer(csym))
	var cerr *C.char
	p := C.grammar_load(cpath, csym, &cerr)
	if p == nil {
		return nil, errors.New(C.GoString(cerr))
	}
	return sitter.NewLanguage(unsafe.Pointer(p)), nil
}
