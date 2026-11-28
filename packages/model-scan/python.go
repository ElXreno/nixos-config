package main

import (
	"regexp"
	"strings"
)

var pyRisky = regexp.MustCompile(`\bsubprocess\b|\bos\.(?:system|popen|exec\w*|spawn\w*|fork|remove|unlink)\b|\bpty\.spawn\b|(?:^|[^.\w])(?:exec|eval|compile)\s*\(|__import__|\bimportlib\b|\bmarshal\.loads?\b|\bpickle\.loads?\b|\bdill\b|\bctypes\b|\bsocket\.|\burllib\b|\brequests\.|\bhttpx\b|\bhttp\.client\b|\baiohttp\b|\bbase64\.\w*decode\b|\bcodecs\.decode\b|\bzlib\.decompress\b|\b__builtins__\b|\bglobals\s*\(\s*\)|\bshutil\.rmtree\b|\bsnapshot_download\b|\bhf_hub_download\b|\btrust_remote_code\s*=\s*True\b`)

func scanPython(t *target, name string, src []byte) {
	t.add(info, name, "Python code: trust_remote_code runs it, read it first")
	hits := 0
	for i, l := range strings.Split(string(src), "\n") {
		if !pyRisky.MatchString(l) {
			continue
		}
		if hits < 20 {
			t.add(info, name, "line %d: %s", i+1, clip(strings.TrimSpace(l), 140))
		}
		hits++
	}
	if hits > 20 {
		t.add(info, name, "%d more lines with risky calls", hits-20)
	}
	checkText(t, name, string(src), false)
}
