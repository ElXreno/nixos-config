package edit

import (
	"strings"
	"testing"
	"unicode/utf8"

	"agent-guard/internal/call"
	"agent-guard/internal/treesitter"
	"agent-guard/internal/verdict"
)

func TestComments(t *testing.T) {
	wall := strings.Join([]string{
		"    # Пропускная способность масштабируется ТОЛЬКО репликами: prefetch у воркера",
		"    # равен единице не из осторожности, а из-за транспорта, faststream держит",
		"    # один лок продюсера на все RPC к шлюзу, поэтому на реплике идёт ровно одна",
		"    # заявка за раз.",
		"    replicas: 4",
	}, "\n")
	cases := []struct {
		name string
		in   call.Change
		want verdict.Level
	}{
		{"values wall", edit("/r/charts/x/values-dev.yaml", "    replicas: 8", wall), verdict.Deny},
		{"values pointer", edit("/r/values.yaml", "", "replicas: 4 # enabled for the decode backlog"), verdict.Silent},
		{"values one line above", edit("/r/values.yaml", "", "# workaround for https://github.com/x/y/issues/1\nfoo: true"), verdict.Silent},
		{"values two lines above", edit("/r/values.yaml", "", "# first line of story\n# second line of story\nfoo: true"), verdict.Deny},
		{"flux image policy marker", edit("/r/hr.yaml", "", `tag: "1.2.3" # {"$imagepolicy": "flux:app:tag"}`), verdict.Silent},
		{"yaml schema directive", write("/r/x.yaml", "# yaml-language-server: $schema=https://x/y.json\nkey: v\n"), verdict.Silent},
		{"commented-out config block", edit("/r/values.yaml", "", "# resources:\n#   limits:\n#     memory: 1Gi\nreplicas: 1"), verdict.Silent},
		{"long single line", edit("/r/values.yaml", "", "replicas: 4 # "+strings.Repeat("word ", 30)), verdict.Deny},
		{"go doc comment", edit("/r/x.go", "", "// Analyze parses src as bash and returns every simple\n// command reachable from it, nested payloads included.\nfunc Analyze(src string) *Analysis {"), verdict.Silent},
		{"go story block", edit("/r/x.go", "", "// We stopped the workers on 16.08 because the queue\n// had 5000 duplicates; 44% were the same job.\n// Now we drain them.\nwork(4)"), verdict.Deny},
		{"go two-line why", edit("/r/x.go", "", "// gopls cannot see the module from the repo root,\n// so resolve go.mod upwards first.\nroot := findRoot(dir)"), verdict.Silent},
		{"go directive", edit("/r/x.go", "", "//go:build linux\n//nolint:errcheck\npackage x"), verdict.Silent},
		{"python budget", edit("/r/x.py", "", "# one\n\n# two\n\n# three\n\nx = 1"), verdict.Deny},
		{"shell shebang and shellcheck", write("/r/x.sh", "#!/usr/bin/env bash\n# shellcheck disable=SC2086\nset -euo pipefail\n"), verdict.Silent},
		{"markdown ignored", write("/r/README.md", strings.Repeat("# heading\n", 5)), verdict.Silent},
		{"multiedit wall", edits("/r/values.yaml", call.Pair{Old: "a: 1", New: "# story line one\n# story line two\na: 2"}), verdict.Deny},
		{"kept lines from old_string do not count", edit("/r/values.yaml", "# old one\n# old two\na: 1", "# old one\n# old two\na: 2"), verdict.Silent},
		{"nix one-liner", edit("/r/x.nix", "", "  # upstream default breaks bwrap, see #1234\n  enable = false;"), verdict.Silent},
		{"go doc reworded, declaration unchanged", edit("/r/x.go", "// Subcategories lists children.\nfunc Subcategories(page string) []string {", "// Subcategories extracts child category slugs from a category page. A\n// page without a tree yields nothing, which the caller treats as a\n// leaf.\nfunc Subcategories(page string) []string {"), verdict.Silent},
		{"kernel block continued mid-block", edit("/r/mm/slub.c", "\t/*\n\t * old text\n\t */", "\t/*\n\t * The objects we did not claim are the live ones, and nothing can be\n\t * freed while the node lock is held, so the count below is exact\n\t * for the whole pass.\n\t */"), verdict.Silent},
		{"c pointer deref is code", edit("/r/x.c", "", "*out = 1;\n*err = 2;\n*n = 3;\n*m = 4;"), verdict.Silent},
		{"rust doc before attribute", edit("/r/x.rs", "", "/// What the cooler does when the host disappears.\n///\n/// Sent once per connection.\n#[derive(Debug)]\npub enum Fallback {"), verdict.Silent},
		{"rust inner doc", write("/r/x.rs", "//! A colour picker, drawn rather than typed.\n//!\n//! Wheel plus a strip.\n\nuse std::io;\n"), verdict.Silent},
		{"kernel block comment", edit("/r/mm/slub.c", "", "\t/*\n\t * The freelist is only stable while both locks are held, and the\n\t * caller may sleep.\n\t */\n\tlockdep_assert_held(&n->list_lock);"), verdict.Silent},
		{"c line comment story", edit("/r/tool.c", "", "// Which slab caches pin otherwise-free pageblocks?\n// We walk every zone and count.\n// Then print the table.\nint main(void) {"), verdict.Deny},
		{"note prose is not commented-out config", edit("/r/values.yaml", "", "# Note: the exporter only refreshes when the upstream token is valid\n# and the scheduler has not been paused for the night\nfoo: 1"), verdict.Deny},
		{"go story block", edit("/r/x.go", "", "// We stopped the workers on 16.08 because the queue\n// had 5000 duplicates; 44% were the same job.\n// Now we drain them.\nwork(4)"), verdict.Deny},
		{"nix string holding c preprocessor lines", write("/r/idle-test.nix", "{ pkgs }:\npkgs.writeCBin \"wakeups\" ''\n  #define _GNU_SOURCE\n  #include <stdio.h>\n  #include <stdlib.h>\n  #include <time.h>\n  int main(void) { return 0; }\n''\n"), verdict.Silent},
		{"nix string holding css ids and colours", edit("/r/firefox/default.nix", "", "  userChrome = ''\n    #urlbar .urlbarView-row[selected] > .urlbarView-row-inner,\n    #urlbar .urlbarView-row:hover > .urlbarView-row-inner,\n    #urlbar .urlbarView-row-inner:focus {\n      color: #ff0000;\n    }\n  '';"), verdict.Silent},
		{"nix story wall still blocked", edit("/r/x.nix", "", "  # We moved the C source out because the hook\n  # tripped on it, and the build then needed\n  # a second file.\n  enable = true;"), verdict.Deny},
		{"nix string with shell story still blocked", edit("/r/x.nix", "", "  script = ''\n    # first we wait for the socket, because the daemon\n    # races the unit on boot and loses the first\n    # connection otherwise\n    sleep 1\n  '';"), verdict.Deny},
		{"toml glued-hash prose still blocked", edit("/r/fml.toml", "", "#Should we control the window. Disabling this disables\n#the loading screen as well.\nearlyWindowControl = true"), verdict.Deny},
		{"todo marker does not excuse a wall", edit("/r/x.go", "", "// TODO(x): we stopped the workers because the queue\n// had 5000 duplicates and 44% were the same job\n// so now we drain them first\nwork(4)"), verdict.Deny},
		{"todo marker does not excuse a long line", edit("/r/x.go", "", "// TODO(x): "+strings.Repeat("story ", 40)+"\nwork(4)"), verdict.Deny},
		{"a run of todo lines is a wall", edit("/r/x.go", "", strings.Repeat("// TODO(x): another paragraph of history\n", 8)+"work(4)"), verdict.Deny},
		{"single todo line is fine", edit("/r/x.go", "", "// TODO(x): drop once upstream #123 lands\nwork(4)"), verdict.Silent},
		{"six-line doc above a declaration", edit("/r/x.go", "", strings.Repeat("// Drain empties the queue before the workers restart.\n", 6)+"func Drain() {"), verdict.Silent},
		{"seven-line doc above a declaration is a wall", edit("/r/x.go", "", strings.Repeat("// We stopped the workers on 16.08 because the queue had duplicates.\n", 7)+"func Drain() {"), verdict.Deny},
		{"long kernel block comment stays allowed", edit("/r/mm/slub.c", "", "/*\n"+strings.Repeat(" * The freelist is only stable while both locks are held.\n", 10)+" */\nstatic void drain(void)"), verdict.Silent},
	}
	for _, prefix := range []string{"# ", "# x"} {
		v := Comments(edit("/r/values.yaml", "", prefix+"Пропускная способность масштабируется только репликами, потому что\n# второй\nreplicas: 4"), Options{})
		if len(v) == 0 || !utf8.ValidString(v[0].Message) {
			t.Errorf("sample after %q must be cut on a rune boundary: %q", prefix, v)
		}
	}
	for _, tc := range cases {
		got := Comments(tc.in, Options{}).Worst()
		if got != tc.want {
			msg := ""
			if v := Comments(tc.in, Options{}); len(v) > 0 {
				msg = v[0].Message
			}
			t.Errorf("%s: want %v got %v\n  %s", tc.name, tc.want, got, msg)
		}
	}
}

type commentCase struct {
	name string
	in   call.Change
	want verdict.Level
}

func checkCases(t *testing.T, mode string, g treesitter.Grammars, cases []commentCase) {
	t.Helper()
	for _, tc := range cases {
		v := Comments(tc.in, Options{Grammars: g})
		if got := v.Worst(); got != tc.want {
			msg := ""
			if len(v) > 0 {
				msg = v[0].Message
			}
			t.Errorf("%s: %s: want %v got %v\n  %s", mode, tc.name, tc.want, got, msg)
		}
	}
}

func lines(ls ...string) string {
	return strings.Join(ls, "\n") + "\n"
}

var (
	apacheGo = lines(
		"// Copyright 2024 Example Corp.",
		"//",
		"// Licensed under the Apache License, Version 2.0 (the \"License\");",
		"// you may not use this file except in compliance with the License.",
		"// You may obtain a copy of the License at",
		"//",
		"//     http://www.apache.org/licenses/LICENSE-2.0",
		"//",
		"// Unless required by applicable law or agreed to in writing, software",
		"// distributed under the License is distributed on an \"AS IS\" BASIS,",
		"// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.",
		"// See the License for the specific language governing permissions and",
		"// limitations under the License.",
		"",
		"package queue",
	)
	mitPython = lines(
		"# Copyright (c) 2024 Example Corp.",
		"#",
		"# Permission is hereby granted, free of charge, to any person obtaining a copy",
		"# of this software and associated documentation files (the \"Software\"), to deal",
		"# in the Software without restriction, including without limitation the rights",
		"# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell",
		"# copies of the Software, and to permit persons to whom the Software is",
		"# furnished to do so, subject to the following conditions:",
		"#",
		"# The above copyright notice and this permission notice shall be included in all",
		"# copies or substantial portions of the Software.",
		"#",
		"# THE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR",
		"# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,",
		"# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE",
		"# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER",
		"# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,",
		"# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE",
		"# SOFTWARE.",
		"",
		"import os",
	)
	kernelC = lines(
		"// SPDX-License-Identifier: GPL-2.0-only",
		"/*",
		" * Copyright (c) 2024 Example Corp.",
		" *",
		" * Driver for the example panel.",
		" */",
		"#include <linux/module.h>",
		"",
		"static int x;",
	)
	cddlC = lines(
		"// SPDX-License-Identifier: CDDL-1.0",
		"/*",
		" * CDDL HEADER START",
		" *",
		" * The contents of this file are subject to the terms of the",
		" * Common Development and Distribution License (the \"License\").",
		" * You may not use this file except in compliance with the License.",
		" *",
		" * You can obtain a copy of the license at usr/src/OPENSOLARIS.LICENSE",
		" * or https://opensource.org/licenses/CDDL-1.0.",
		" * See the License for the specific language governing permissions",
		" * and limitations under the License.",
		" *",
		" * CDDL HEADER END",
		" */",
		"",
		"/*",
		" * Copyright (c) 2024 by Example. All rights reserved.",
		" */",
		"",
		"#include <stdio.h>",
	)
	iscGo = lines(
		"// Copyright (c) 2018, Jane Doe",
		"//",
		"// Permission to use, copy, modify, and/or distribute this software for any",
		"// purpose with or without fee is hereby granted, provided that the above",
		"// copyright notice and this permission notice appear in all copies.",
		"//",
		"// THE SOFTWARE IS PROVIDED \"AS IS\" AND THE AUTHOR DISCLAIMS ALL WARRANTIES",
		"// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF",
		"// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR",
		"// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES",
		"// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN",
		"// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF",
		"// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.",
		"",
		"package x",
	)
	llvmCpp = lines(
		"//===- Foo.cpp - Foo pass ------------------------------------*- C++ -*-===//",
		"//",
		"// Part of the LLVM Project, under the Apache License v2.0 with LLVM Exceptions.",
		"// See https://llvm.org/LICENSE.txt for license information.",
		"// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception",
		"//",
		"//===----------------------------------------------------------------------===//",
		"//",
		"// This file implements the Foo pass.",
		"//",
		"//===----------------------------------------------------------------------===//",
		"",
		"#include \"Foo.h\"",
	)
)

func TestLicenseHeaders(t *testing.T) {
	cases := []commentCase{
		{"apache header above a go package", write("/r/queue.go", apacheGo), verdict.Silent},
		{"mit header above python imports", write("/r/x.py", mitPython), verdict.Silent},
		{"spdx line glued to a c license block", write("/r/panel.c", kernelC), verdict.Silent},
		{"spdx line glued to a cddl block", write("/r/test.c", cddlC), verdict.Silent},
		{"spdx line above rust inner doc", write("/r/x.rs", lines("// SPDX-License-Identifier: MIT", "//! A colour picker, drawn rather than typed.", "//!", "//! Wheel plus a strip.", "", "use std::io;")), verdict.Silent},
		{"go authors header", write("/r/x.go", lines("// Copyright 2009 The Go Authors. All rights reserved.", "// Use of this source code is governed by a BSD-style", "// license that can be found in the LICENSE file.", "", "package x")), verdict.Silent},
		{"story after a copyright line still blocked", write("/r/x.go", lines("// Copyright 2024 Example Corp.", "// We moved this file out of the old daemon because the", "// queue kept duplicating jobs, and the split fixed it.", "// The numbers dropped by 44% after that.", "", "package x")), verdict.Deny},
		{"story smuggled between license lines still blocked", write("/r/x.py", lines("# Copyright 2024 Example", "# we moved this out of the daemon because", "# the queue kept duplicating jobs and the", "# split fixed it after two weeks of pain", "# Licensed under MIT.", "import os")), verdict.Deny},
		{"header without a license anchor still blocked", write("/r/x.py", lines("# This module holds the queue logic we moved out of", "# the daemon after the duplicates, see the old notes,", "# and it is distributed across two packages now.", "import os")), verdict.Deny},
		{"license words inside a function still blocked", write("/r/x.go", lines("package x", "", "func f() {", "\t// Licensed under MIT, but the real story is that we", "\t// moved this from the old daemon because the queue", "\t// kept duplicating jobs.", "\twork(4)", "}")), verdict.Deny},
		{"long line inside a license header still blocked", write("/r/x.py", lines("# Copyright 2024 Example", "# Licensed under MIT, "+strings.Repeat("and the story goes on ", 6), "import os")), verdict.Deny},
		{"isc header", write("/r/x.go", iscGo), verdict.Silent},
		{"llvm banner header with a description", write("/r/foo.cpp", llvmCpp), verdict.Silent},
		{"long spdx copyright notice in a block header", write("/r/nv.c", lines("/*", " * SPDX-FileCopyrightText: Copyright (c) 2023-2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.", " * SPDX-License-Identifier: MIT", " */", "", "#include <linux/module.h>")), verdict.Silent},
		{"jsdoc glued to an spdx line", write("/r/x.ts", lines("// SPDX-License-Identifier: MIT", "/**", " * Entry point of the CLI.", " * Parses flags and dispatches.", " * Exits non-zero on failure.", " */", "import { run } from \"./run\";")), verdict.Silent},
		{"jsdoc after an spdx line", write("/r/x.ts", lines("// SPDX-License-Identifier: MIT", "", "/**", " * Entry point of the CLI.", " * Parses flags and dispatches.", " * Exits non-zero on failure.", " */", "import { run } from \"./run\";")), verdict.Silent},
		{"jsdoc carrying a copyright line", write("/r/x.ts", lines("/**", " * Copyright 2020 Foo", " * Description line 1 of this file", " * Description line 2 of this file", " * Description line 3 of this file", " */", "foo();")), verdict.Silent},
		{"one license word does not carry a story", write("/r/x.py", lines("# Copyright 2024 Example", "# We moved the cache next to the binary because the sandbox", "# hides the home directory, and the old layout was not", "# permitted by the daemon, so the lookup is relative now", "# and the startup check rebuilds it after a crash.", "# See the incident notes.", "import os")), verdict.Deny},
		{"story under a header in its own block still blocked", write("/r/x.py", lines("# Copyright 2024 Example", "", "# Permission bits get dropped by the sandbox, so we chmod on start.", "# The daemon has no way to write elsewhere before login, and the", "# old approach damaged the lockfile twice, so the cache is rebuilt.", "import os")), verdict.Deny},
		{"prose paragraph inside a license block still blocked", write("/r/x.go", strings.Replace(apacheGo, "\n\npackage queue", "\n//\n// We moved the queue here after the duplicates showed up,\n// the old daemon kept two copies of every job, and the\n// split fixed it, so the counters are trustworthy again\n// and nobody should merge the packages back.\n\npackage queue", 1)), verdict.Deny},
		{"apache block comment with blank lines between paragraphs", write("/r/policy.go", lines("/*", "Copyright 2014 The Kubernetes Authors.", "", "Licensed under the Apache License, Version 2.0 (the \"License\");", "you may not use this file except in compliance with the License.", "You may obtain a copy of the License at", "", "    http://www.apache.org/licenses/LICENSE-2.0", "", "Unless required by applicable law or agreed to in writing, software", "distributed under the License is distributed on an \"AS IS\" BASIS,", "WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.", "See the License for the specific language governing permissions and", "limitations under the License.", "*/", "", "package policy")), verdict.Silent},
		{"webm header with a patent grant", write("/r/x.js", lines("// Copyright (c) 2010 The WebM project authors. All Rights Reserved.", "//", "// Use of this source code is governed by a BSD-style license", "// that can be found in the LICENSE file in the root of the source", "// tree. An additional intellectual property rights grant can be found", "// in the file PATENTS.  All contributing project authors may", "// be found in the AUTHORS file in the root of the source tree.", "", "export const x = 1;")), verdict.Silent},
		{"unlicense dedication", write("/r/x.py", lines("# This is free and unencumbered software released into the public domain.", "#", "# Anyone is free to copy, modify, publish, use, compile, sell, or", "# distribute this software, either in source code form or as a compiled", "# binary, for any purpose, commercial or non-commercial, and by any", "# means.", "#", "# For more information, please refer to <https://unlicense.org>", "import os")), verdict.Silent},
		{"story glued under a license block still blocked", write("/r/x.go", strings.Replace(apacheGo, "\n\npackage queue", "\n// We moved the queue here after the duplicates showed up,\n// the old daemon kept two copies of every job,\n// and the split fixed it.\n\npackage queue", 1)), verdict.Deny},
		{"copyright-shaped story lines after code still blocked", write("/r/x.go", lines("package x", "", "func F() {}", "", "// Copyright 2024 we moved this because of the queue duplicates", "// Copyright 2024 and the numbers dropped by 44% after that", "// Copyright 2024 so nobody should merge the packages back", "// Copyright 2024 until the daemon is gone")), verdict.Deny},
		{"overlong copyright line still blocked", write("/r/x.go", lines("// Copyright 2024 "+strings.Repeat("x", 200), "package x")), verdict.Deny},
		{"common words near a license do not make a header", write("/r/x.py", lines("# SPDX-License-Identifier: MIT", "# Permission model: workers drop privileges after fork,", "# and a damaged lockfile is rebuilt rather than repaired,", "# which is permitted because the cache is disposable,", "# and distributed builds never share it.", "# Contributors should keep it that way.", "import os")), verdict.Deny},
		{"product names near a copyright do not make a header", write("/r/x.nix", lines("# Copyright 2024 x", "# We moved the apache vhost out of the module because the", "# mozilla sync endpoint kept 502ing behind it, and the split", "# fixed it. FreeBSD tests are unaffected.", "{ }: { }")), verdict.Deny},
		{"copyright word opening a long story line", write("/r/x.go", lines("// Copyright note: "+strings.Repeat("we moved this because the queue duplicated jobs ", 3), "package x")), verdict.Deny},
		{"copyrighted prose is not a copyright notice", write("/r/x.go", lines("package x", "", "func f() {", "\t// Copyrighted material was stripped from the fixtures because", "\t// the upstream test suite shipped scans of a paid manual, and", "\t// we only noticed after the release.", "\twork(4)", "}")), verdict.Deny},
		{"note line glued to a c story block keeps the old verdict", write("/r/x.c", lines("int g;", "", "// Note: x", "/*", " * we stopped the workers because the queue had duplicates", " * and 44% were the same job, so now we drain them first", " * and the counters line up again", " */", "int main(void) { return 0; }")), verdict.Deny},
	}
	checkCases(t, "heuristic", nil, cases)
	checkCases(t, "syntax", testGrammars(t), cases)
}

func TestMemoryFilesAreIgnored(t *testing.T) {
	wall := "# one\n# two\n# three\n# four\nx = 1\n"
	match := MemoryMatcher([]string{"/h/.claude/projects/*/memory/"})
	opts := Options{Memory: func(p string) bool { return match(p, "") }}
	if got := Comments(write("/h/.claude/projects/-x/memory/note.py", wall), opts).Worst(); got != verdict.Silent {
		t.Errorf("a memory file is not checked, got %v", got)
	}
	if got := Comments(write("/h/notes/note.py", wall), opts).Worst(); got != verdict.Deny {
		t.Errorf("control: the same wall elsewhere is denied, got %v", got)
	}
}

func TestWriteAgainstDisk(t *testing.T) {
	old := lines(
		"import os",
		"",
		"# The cache lives next to the binary because the sandbox",
		"# hides the home directory, and the daemon has no other",
		"# writable path before the first user logs in, so keep",
		"# the lookup relative.",
		"CACHE = os.path.dirname(__file__)",
	)
	story := lines("# We moved the cache here after the duplicates,", "# and the numbers dropped by 44%,", "# so keep it.")
	cases := func(t *testing.T) []commentCase {
		existing := onDisk(t, "cache.py", old)
		plain := onDisk(t, "plain.py", "import os\n")
		separators := onDisk(t, "sep.py", "import os\n#\nx = 1\n#\n")
		inline := onDisk(t, "inline.py", "import os\nx = 1  # enabled for the decode backlog\n")
		wall := strings.Join(strings.Split(old, "\n")[2:6], "\n") + "\n"
		return []commentCase{
			{"write keeps the comments already on disk", write(existing, old+"TIMEOUT = 5\n"), verdict.Silent},
			{"write copying an existing wall elsewhere still blocked", write(existing, old+"\n"+wall+"TIMEOUT = 5\n"), verdict.Deny},
			{"write keeping an inline comment that is on disk", write(inline, "import os\nx = 1  # enabled for the decode backlog\ny = 2  # enabled for the decode backlog\n# one\n# two\n"), verdict.Silent},
			{"empty comment lines on disk do not excuse a new wall", write(separators, "import os\n#\nx = 1\n# first line of a story\n#\n# second line of a story\ny = 2\n"), verdict.Deny},
			{"write adding a story to an existing file still blocked", write(plain, "import os\n"+story+"TIMEOUT = 5\n"), verdict.Deny},
			{"write to a missing path is judged as new", write("/r/missing.py", old), verdict.Deny},
		}
	}
	checkCases(t, "heuristic", nil, cases(t))
	checkCases(t, "syntax", testGrammars(t), cases(t))

	existing := onDisk(t, "cache.py", old)
	for _, g := range []treesitter.Grammars{nil, testGrammars(t)} {
		replayed := Options{Grammars: g, Previous: Missing}
		if got := Comments(write(existing, old+"TIMEOUT = 5\n"), replayed).Worst(); got != verdict.Deny {
			t.Errorf("grammars %v: a replayed write is judged as new, want deny got %v", g != nil, got)
		}
	}
}

func TestDenyMessageListsLines(t *testing.T) {
	content := lines(
		"import os",
		"",
		"# We moved the cache here after the duplicates,",
		"# and the numbers dropped by 44%,",
		"# so keep it.",
		"x = 1",
		"",
		"y = 2  # "+strings.Repeat("story ", 20),
	)
	v := Comments(write("/r/x.py", content), Options{})
	if len(v) == 0 {
		t.Fatal("expected a verdict")
	}
	msg := v[0].Message
	for _, want := range []string{"Blocked: comment wall in source files (", "L3-L5", "L8 comment line of 121 characters (limit 100)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}

	var many []string
	for range 10 {
		many = append(many, "# first line of a story", "# second line of it", "# third line of it", "x = 1")
	}
	v = Comments(write("/r/x.py", lines(many...)), Options{})
	if len(v) == 0 || !strings.Contains(v[0].Message, "+2 more: L33-L35, L37-L39") {
		t.Errorf("ten walls must list eight and point at the rest: %v", v)
	}

	v = Comments(write("/r/x.py", lines("# one", "", "# two", "", "# three", "", "x = 1")), Options{})
	if len(v) == 0 || strings.Contains(v[0].Message, "Fix every listed line") {
		t.Errorf("a budget-only verdict lists no lines to fix: %v", v)
	}

	v = Comments(edit("/r/x.py", "", "# one story line\n# two story line\n# three story line\nx = 1"), Options{})
	if len(v) == 0 || strings.Contains(v[0].Message, "L1") {
		t.Errorf("an Edit has no file line numbers to cite: %v", v)
	}
}

func comment(prefix string, n int) string {
	return prefix + strings.Repeat("w", n-len(prefix))
}

func TestLongLineAdviseOnWrite(t *testing.T) {
	long := comment("// long pointer ", 113)
	cases := []commentCase{
		{"write with one long line", write("/r/x.go", lines("package x", "", long, "func F() {}")), verdict.Advise},
		{"write with a line over 120", write("/r/x.go", lines("package x", "", comment("// long pointer ", 135), "func F() {}")), verdict.Deny},
		{"write with four long lines", write("/r/x.go", lines("package x", "", long, "var a = 1", long+"b", "var b = 1", long+"c", "var c = 1", long+"d", "var d = 1")), verdict.Deny},
		{"write with a long line and a wall", write("/r/x.go", lines("package x", "", long, "var a = 1", "", "func F() {", "\t// one", "\t// two", "\t// three", "\twork()", "}")), verdict.Deny},
		{"edit with one long line", edit("/r/x.go", "func F() {}", long+"\nfunc F() {}"), verdict.Deny},
		{"config write with one long line", write("/r/values.yaml", lines(comment("# long pointer ", 113), "replicas: 1")), verdict.Deny},
		{"write with a long dated line", write("/r/x.go", lines("package x", "", comment("// measured 2026-08 on the box ", 113), "func F() {}")), verdict.Deny},
	}
	checkCases(t, "heuristic", nil, cases)
	checkCases(t, "syntax", testGrammars(t), cases)

	v := Comments(cases[0].in, Options{})
	if len(v) == 0 || !strings.Contains(v[0].Message, "L3 comment line of 113 characters") || strings.Contains(v[0].Message, "Blocked") {
		t.Errorf("advice must cite the line and its length without blocking: %v", v)
	}
}

func TestScratchCommentsAdvise(t *testing.T) {
	wall := lines("# first line of a story", "# second line of it", "# third line of it", "x = 1")
	cases := []struct {
		name    string
		in      call.Change
		scratch string
		want    verdict.Level
	}{
		{"wall written under scratch", write("/mnt/scratch/claude-code/p/2026-11-09-x-ab12/sim.py", wall), "/mnt/scratch/claude-code", verdict.Advise},
		{"wall edited under scratch", edit("/mnt/scratch/claude-code/p/sim.py", "x = 0", wall), "/mnt/scratch/claude-code", verdict.Advise},
		{"wall in a repo", write("/r/sim.py", wall), "/mnt/scratch/claude-code", verdict.Deny},
		{"sibling of the scratch dir", write("/mnt/scratch/claude-code-old/sim.py", wall), "/mnt/scratch/claude-code", verdict.Deny},
		{"no scratch configured", write("/mnt/scratch/claude-code/p/sim.py", wall), "", verdict.Deny},
	}
	for _, tc := range cases {
		v := Comments(tc.in, Options{Scratch: tc.scratch})
		if got := v.Worst(); got != tc.want {
			t.Errorf("%s: want %v got %v", tc.name, tc.want, got)
		}
		if tc.want == verdict.Advise && (len(v) == 0 || strings.Contains(v[0].Message, "Blocked") || !strings.Contains(v[0].Message, "3 consecutive comment lines")) {
			t.Errorf("%s: scratch advice must list the problems without blocking: %v", tc.name, v)
		}
	}
}

func checksOf(v verdict.Verdicts) string {
	var out []string
	for _, x := range v {
		out = append(out, x.Level.String()+":"+x.Check)
	}
	return strings.Join(out, ",")
}

func TestStoryAndDateAdvice(t *testing.T) {
	cases := []struct {
		name string
		in   call.Change
		want string
	}{
		{"history phrase", write("/r/x.py", "# used to be a map, now a slice\nx = 1\n"), "advise:comment-story"},
		{"present state is fine", write("/r/x.py", "# the username is no longer a pointer\nx = 1\n"), ""},
		{"hyphenated previously is fine", write("/r/x.py", "# reuse the previously-generated id\nx = 1\n"), ""},
		{"temporary status", write("/r/x.py", "# temporarily disabled until upstream lands\nx = 1\n"), "advise:comment-story"},
		{"phrase in backticks", write("/r/x.py", "# matches `used to be` in prose\nx = 1\n"), ""},
		{"phrase on a url line", write("/r/x.py", "# https://x/y/used-to-be used to be\nx = 1\n"), ""},
		{"phrase in a string", write("/r/x.py", "x = 'used to be'\n"), ""},
		{"regression label", write("/r/x.py", "# regression: the cache dropped writes\nx = 1\n"), "advise:comment-story"},
		{"migration file", write("/r/migrations/0003_rename.py", "# renamed from foo\nx = 1\n"), ""},
		{"iso month", write("/r/x.py", "# measured 2026-08 on KURWA\nx = 1\n"), "advise:comment-date"},
		{"month and year", write("/r/x.py", "# since March 2026\nx = 1\n"), "advise:comment-date"},
		{"russian month and year", write("/r/x.py", "# с марта 2026 так\nx = 1\n"), "advise:comment-date"},
		{"copyright years", write("/r/x.py", "# Copyright 2023-2025 Example\nx = 1\n"), ""},
		{"date in a url", write("/r/x.py", "# see https://x/2026-08-01.html\nx = 1\n"), ""},
		{"date in a string", write("/r/x.py", "x = \"2026-07-01\"\n"), ""},
		{"version tag", write("/r/x.py", "# v2024-01 schema\nx = 1\n"), ""},
		{"repo named like a compat layer", write("/home/u/repos/compat-layer/src/x.py", "# this used to be a map\nx = 1\n"), "advise:comment-story"},
		{"file named compatible", write("/r/compatible.py", "# this used to be a map\nx = 1\n"), "advise:comment-story"},
		{"compat shim file", write("/r/compat.py", "# this used to be a map\nx = 1\n"), ""},
		{"dated metadata line", write("/r/x.py", "# Created: 2026-08-01\nx = 1\n"), "advise:comment-date"},
		{"short regression label", write("/r/x.go", "package x\n\n// Regression: cache dropped writes\nvar x = 1\n"), "advise:comment-story"},
		{"may as a verb before a number", write("/r/x.go", "package x\n\n// the buffer may 2048 entries\nvar x = 1\n"), ""},
		{"decimal abbreviation before a number", write("/r/x.go", "package x\n\n// ports dec 2000\nvar x = 1\n"), ""},
		{"russian word containing a month stem", write("/r/x.py", "# маяк 2048 байт, смартфон 2020 года\nx = 1\n"), ""},
		{"capitalized month abbreviation", write("/r/x.py", "# broke in Dec 2025\nx = 1\n"), "advise:comment-date"},
		{"date kept from old_string", edit("/r/x.py", "# measured 2026-08\nx = 1", "# measured 2026-08\nx = 2"), ""},
	}
	for _, tc := range cases {
		for _, g := range []treesitter.Grammars{nil, testGrammars(t)} {
			if got := checksOf(Comments(tc.in, Options{Grammars: g})); got != tc.want {
				t.Errorf("%s (grammars %v): want %q got %q", tc.name, g != nil, tc.want, got)
			}
		}
	}
}
