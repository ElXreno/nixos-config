package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type rule struct {
	sev  sev
	what string
	re   *regexp.Regexp
}

var textRules = []rule{
	{crit, "IOC from the malspark GGUF report", regexp.MustCompile(`(?i)technoarteng|olliE-rx0-seed|rat_plugin_command|ollie_cam|ollie[/\\]rat\.(?:conf|log)`)},
	{crit, "file-dropper API", regexp.MustCompile(`(?i)IO\.File\]::|IO\.Compression|DeflateStream|WriteAllBytes|FromBase64String|Reflection\.Assembly\]::Load`)},
	{crit, "Windows execution primitive", regexp.MustCompile(`(?i)Start-Process\b|-WindowStyle\s+Hidden|-ExecutionPolicy\s+Bypass|-EncodedCommand\b|Invoke-WebRequest|Invoke-Expression|\biex\s*[(\$]|DownloadString|DownloadFile|Net\.WebClient|Add-MpPreference|\bcmd(?:\.exe)?\s+/[ck]\b|\bmshta\b|\brundll32\b|\bregsvr32\b|\bcertutil\s+-|\bbitsadmin\b|schtasks\s+/create`)},
	{crit, "hidden instruction to the model", regexp.MustCompile(`(?i)\[\s*SYSTEM\s+(?:DIRECTIVE|OVERRIDE|INSTRUCTION)S?\s*\]|\bMUST\s+(?:always\s+)?(?:call|use|invoke|run|execute)\s+(?:the\s+)?[\w.-]+\s+(?:tool|function|command)|\bNEVER\s+(?:mention|reveal|disclose)\s+(?:them|this|it|these|the\s+(?:rules?|directives?|instructions?))\b|\b(?:do\s+not|don'?t)\s+(?:mention|reveal|disclose)\s+(?:this|these|them|the\s+(?:rules?|directives?|instructions?|commands?))\b|without\s+(?:telling|informing|notifying|asking)\s+the\s+user|\b(?:silently|secretly|covertly|quietly)\s+(?:run|execute|call|add|insert|append|send|include)\b|\brun\s+(?:it\s+)?in\s+(?:the\s+)?background\b`)},
	{high, "instruction override", regexp.MustCompile(`(?i)\bignore\s+(?:all\s+)?(?:the\s+)?(?:previous|prior|above|earlier)\s+(?:instructions|guidelines|rules|messages)|\bdisregard\s+(?:all\s+)?(?:the\s+)?(?:previous|prior|above)`)},
	{high, "shell command", regexp.MustCompile(`(?i)\b(?:powershell|pwsh)(?:\.exe)?\b|\bcurl\s+(?:-[A-Za-z-]+\s+)*https?://|\bwget\s+(?:-[A-Za-z-]+\s+)*https?://|\bnc\s+-[A-Za-z]*[el]\b|\bbash\s+-[ci]\b|\bsh\s+-c\b|chmod\s+\+x|/bin/(?:ba|z|da)?sh\b|/dev/tcp/|\bpython3?\s+-c\b|base64\s+(?:-d|--decode)\b|\bmkfifo\b|\bcrontab\b|authorized_keys|\.bashrc\b|\bnohup\b|\|\s*(?:ba)?sh\b`)},
	{high, "sensitive path or wallet", regexp.MustCompile(`(?i)/etc/(?:passwd|shadow|sudoers)\b|\.ssh/|\bid_(?:rsa|ed25519|ecdsa)\b|\.aws/credentials|\.netrc\b|\.git-credentials|\.docker/config\.json|%(?:TEMP|APPDATA|LOCALAPPDATA)%|\\Windows\\Temp|AppData\\|\bmetamask\b|seed\s+phrase|wallet\.dat`)},
	{high, "C2 or miner indicator", regexp.MustCompile(`(?i)\.onion\b|stratum\+tcp|\bxmrig\b|\bmonero\b|\brandomx\b|SocksPort\s+\d`)},
}

var (
	urlRe = regexp.MustCompile(`(?i)\b(?:https?|ftps?)://[^\s"'<>)\]\\]+`)
	ipRe  = regexp.MustCompile(`\b(?:25[0-5]|2[0-4]\d|1?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|1?\d?\d)){3}\b`)
	b64Re = regexp.MustCompile(`[A-Za-z0-9+/]{160,}={0,2}`)
	hexRe = regexp.MustCompile(`(?i)(?:\\x[0-9a-f]{2}){32,}|\b[0-9a-f]{256,}\b`)
)

func benignURL(u string) bool {
	l := strings.ToLower(u)
	for _, h := range []string{"://example.com", "://example.org", "://www.example.com", "://json-schema.org"} {
		if strings.Contains(l, h) {
			return true
		}
	}
	return false
}

func checkText(t *target, file, s string, tpl bool) {
	checkUnicode(t, file, s)
	for _, r := range textRules {
		for _, m := range r.re.FindAllString(s, 3) {
			t.add(r.sev, file, "%s: %q", r.what, clip(m, 120))
		}
	}
	if m := b64Re.FindString(s); m != "" {
		t.add(high, file, "base64-like blob of %d characters: %q", len(m), clip(m, 60))
	}
	if m := hexRe.FindString(s); m != "" {
		t.add(med, file, "hex blob of %d characters", len(m))
	}
	if !tpl {
		return
	}
	for _, u := range urlRe.FindAllString(s, 5) {
		if !benignURL(u) {
			t.add(med, file, "URL in a chat template: %q", clip(u, 120))
		}
	}
	for _, ip := range ipRe.FindAllString(s, 3) {
		t.add(med, file, "IP address in a chat template: %s", ip)
	}
}

func checkUnicode(t *target, file, s string) {
	var tags []byte
	var ntags, zw, bidi, vs int
	for i, r := range s {
		switch {
		case r >= 0xE0000 && r <= 0xE007F:
			ntags++
			if c := r - 0xE0000; c >= 0x20 && c < 0x7f {
				tags = append(tags, byte(c))
			}
		case r == 0xFEFF && i == 0:
		case r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF || r == 0x180E || (r >= 0x2060 && r <= 0x2064):
			zw++
		case (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200E || r == 0x200F || r == 0x061C:
			bidi++
		case (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF):
			vs++
		}
	}
	if ntags > 0 {
		t.add(crit, file, "%d invisible Unicode tag characters spell %q", ntags, clip(string(tags), 200))
	}
	if zw > 0 {
		t.add(high, file, "%d zero-width characters", zw)
	}
	if bidi > 0 {
		t.add(high, file, "%d bidirectional control characters", bidi)
	}
	if vs > 8 {
		t.add(high, file, "%d variation selectors, enough to encode hidden bytes", vs)
	}
	if !utf8.ValidString(s) {
		t.add(med, file, "text is not valid UTF-8")
	}
}

var magics = []struct{ sig, label string }{
	{"\x7fELF", "ELF executable"}, {"PK\x03\x04", "ZIP archive"}, {"PK\x05\x06", "ZIP end record"},
	{"\xfd7zXZ\x00", "xz stream"}, {"7z\xbc\xaf\x27\x1c", "7z archive"}, {"Rar!\x1a\x07", "RAR archive"},
	{"\x28\xb5\x2f\xfd", "zstd frame"}, {"\xcf\xfa\xed\xfe", "Mach-O"}, {"%PDF-", "PDF"}, {"\x89PNG\r\n", "PNG"},
	{"GGUF\x03\x00\x00\x00", "nested GGUF"},
}

func analyzeBlob(b []byte) string {
	var parts []string
	if i := findPE(b); i >= 0 {
		parts = append(parts, fmt.Sprintf("Windows PE executable at +%d", i))
	}
	for _, m := range magics {
		if i := bytes.Index(b, []byte(m.sig)); i >= 0 {
			parts = append(parts, fmt.Sprintf("%s signature at +%d", m.label, i))
		}
	}
	switch {
	case bytes.HasPrefix(b, []byte("#!")):
		parts = append(parts, "starts with a shebang")
	case bytes.HasPrefix(b, []byte("\x1f\x8b\x08")):
		parts = append(parts, "starts with a gzip header")
	}
	e := entropy(b)
	desc := fmt.Sprintf("entropy %.2f bits/byte", e)
	if e > 7.5 {
		desc += " (compressed or encrypted)"
	}
	parts = append(parts, desc)
	var runs []string
	for _, m := range regexp.MustCompile(`[\x20-\x7e]{8,}`).FindAll(b, 4) {
		runs = append(runs, strconv.Quote(clip(string(m), 60)))
	}
	if len(runs) > 0 {
		parts = append(parts, "strings "+strings.Join(runs, " "))
	}
	return strings.Join(parts, "; ")
}

func findPE(b []byte) int {
	for i := 0; i < len(b); {
		j := bytes.Index(b[i:], []byte("MZ"))
		if j < 0 {
			return -1
		}
		k := i + j
		if k+64 <= len(b) {
			pe := int(binary.LittleEndian.Uint32(b[k+60:]))
			if pe > 0 && k+pe+4 <= len(b) && string(b[k+pe:k+pe+4]) == "PE\x00\x00" {
				return k
			}
		}
		i = k + 1
	}
	return -1
}

func entropy(b []byte) float64 {
	if len(b) == 0 {
		return 0
	}
	var c [256]int
	for _, x := range b {
		c[x]++
	}
	e := 0.0
	for _, n := range c {
		if n > 0 {
			p := float64(n) / float64(len(b))
			e -= p * math.Log2(p)
		}
	}
	return e
}

type jtok struct {
	kind byte
	body string
}

func lexJinja(s string) ([]jtok, error) {
	var out []jtok
	for len(s) > 0 {
		i := -1
		for _, o := range []string{"{{", "{%", "{#"} {
			if j := strings.Index(s, o); j >= 0 && (i < 0 || j < i) {
				i = j
			}
		}
		if i < 0 {
			out = append(out, jtok{'t', s})
			break
		}
		if i > 0 {
			out = append(out, jtok{'t', s[:i]})
		}
		open := s[i : i+2]
		s = s[i+2:]
		if open == "{#" {
			j := strings.Index(s, "#}")
			if j < 0 {
				return out, fmt.Errorf("unclosed comment")
			}
			out = append(out, jtok{'c', s[:j]})
			s = s[j+2:]
			continue
		}
		closer, kind := "}}", byte('e')
		if open == "{%" {
			closer, kind = "%}", 's'
		}
		j := findClose(s, closer)
		if j < 0 {
			return out, fmt.Errorf("unclosed %s", open)
		}
		body := strings.Trim(s[:j], "-+~ \t\r\n")
		s = s[j+2:]
		if kind == 's' && firstWord(body) == "raw" {
			loc := regexp.MustCompile(`\{%[-+]?\s*endraw\s*[-+]?%\}`).FindStringIndex(s)
			if loc == nil {
				return out, fmt.Errorf("unclosed raw block")
			}
			out = append(out, jtok{'t', s[:loc[0]]})
			s = s[loc[1]:]
			continue
		}
		out = append(out, jtok{kind, body})
	}
	return out, nil
}

func findClose(s, c string) int {
	var q byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if q != 0 {
			switch ch {
			case '\\':
				i++
			case q:
				q = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			q = ch
			continue
		}
		if strings.HasPrefix(s[i:], c) {
			return i
		}
	}
	return -1
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t\r\n("); i >= 0 {
		return s[:i]
	}
	return s
}

type exprInfo struct {
	idents, attrs, strs, filters []string
	escaped                      []string
}

func identStart(c byte) bool { return c == '_' || (c|0x20) >= 'a' && (c|0x20) <= 'z' }

func identChar(c byte) bool { return identStart(c) || c >= '0' && c <= '9' }

func scanExpr(s string) exprInfo {
	var e exprInfo
	prev := byte(0)
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\'' || c == '"':
			str, n, esc := jinjaString(s[i:])
			e.strs = append(e.strs, str)
			if esc {
				e.escaped = append(e.escaped, str)
			}
			if prev == '[' {
				e.attrs = append(e.attrs, str)
			}
			i += n
			prev = 's'
		case identStart(c):
			j := i
			for j < len(s) && identChar(s[j]) {
				j++
			}
			w := s[i:j]
			switch prev {
			case '.':
				e.attrs = append(e.attrs, w)
			case '|':
				e.filters = append(e.filters, w)
			default:
				e.idents = append(e.idents, w)
			}
			i = j
			prev = 'w'
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		default:
			prev = c
			i++
		}
	}
	return e
}

func jinjaString(s string) (string, int, bool) {
	q := s[0]
	var b strings.Builder
	esc := false
	i := 1
	for i < len(s) && s[i] != q {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		c := s[i+1]
		i += 2
		switch c {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'x', 'u', 'U':
			n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[c]
			if i+n <= len(s) {
				if v, err := strconv.ParseUint(s[i:i+n], 16, 32); err == nil {
					b.WriteRune(rune(v))
					i += n
					esc = true
					continue
				}
			}
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), min(i+1, len(s)), esc
}

var (
	sstiCrit = setOf("__class__", "__base__", "__bases__", "__mro__", "__subclasses__", "__globals__", "__builtins__", "__import__",
		"__init__", "__dict__", "__getattribute__", "__reduce__", "__reduce_ex__", "__code__", "__func__", "__self__", "__module__",
		"__loader__", "__spec__", "_module", "_TemplateReference__context", "func_globals", "f_globals", "gi_frame", "popen", "Popen",
		"check_output", "check_call", "getoutput", "eval", "exec", "execfile", "compile", "getattr", "setattr", "delattr", "globals",
		"locals", "vars", "import_module", "subprocess")
	sstiHigh    = setOf("cycler", "joiner", "lipsum", "self", "request", "config", "url_for", "get_flashed_messages", "application")
	sstiStrings = []string{"__class__", "__subclasses__", "__builtins__", "__globals__", "__import__", "__mro__", "__init__", "os.system", "os.popen", "subprocess", "load_module"}
	inspected   = setOf("content", "arguments", "prompt", "query")
	benignLit   = setOf("system", "user", "assistant", "tool", "tools", "function", "developer", "ipython", "model", "thought", "reasoning",
		"reasoning_content", "text", "image", "image_url", "audio", "video", "content", "role", "name", "arguments", "tool_calls",
		"tool_call_id", "parameters", "properties", "required", "description", "type", "object", "string", "integer", "number",
		"boolean", "array", "null", "none", "true", "false", "audio_url", "video_url", "input_audio", "content_blocks", "file",
		"document", "media", "vision", "json", "xml", "python", "thinking", "query", "items", "enum", "default", "title")
	blockOpeners = setOf("if", "for", "macro", "call", "filter", "block", "with", "generation", "autoescape", "trans")
	specialRe    = regexp.MustCompile(`<\|[^|>]{0,40}\|>|<｜[^｜]{0,40}｜>|</?[A-Za-z_][\w:-]{0,40}>|\[/?[A-Z_]{2,20}\]`)
	wordRe       = regexp.MustCompile(`[A-Za-z]{2,}`)
	genBenign    = setOf("assistant", "model", "think", "thinking", "start", "end", "of", "turn", "im", "gpt", "bot", "response",
		"channel", "analysis", "final", "commentary", "message", "reasoning", "answer", "ai", "eos", "bos", "sep", "output")
)

func checkTemplate(t *target, file, text string) {
	checkText(t, file, text, true)
	toks, err := lexJinja(text)
	if err != nil {
		t.add(med, file, "chat template does not lex as Jinja: %v", err)
	}
	var strs, escaped []string
	var stack []string
	genDepth, genOn := -1, false
	var gen strings.Builder
	for _, tk := range toks {
		switch tk.kind {
		case 't':
			if genOn {
				gen.WriteString(tk.body)
			}
			continue
		case 'c':
			continue
		}
		e := scanExpr(tk.body)
		strs = append(strs, e.strs...)
		escaped = append(escaped, e.escaped...)
		ssti(t, file, e)
		if tk.kind == 'e' {
			if genOn {
				for _, s := range e.strs {
					gen.WriteString(s)
					gen.WriteByte(' ')
				}
			}
			continue
		}
		w := firstWord(tk.body)
		switch {
		case w == "if":
			stack = append(stack, w)
			if genDepth < 0 && contains(e.idents, "add_generation_prompt") && !contains(e.idents, "not") {
				genDepth, genOn = len(stack), true
			}
			logicBomb(t, file, tk.body, e)
		case w == "elif":
			logicBomb(t, file, tk.body, e)
			if len(stack) == genDepth {
				genOn = false
			}
		case w == "else":
			if len(stack) == genDepth {
				genOn = false
			}
		case w == "set" && !strings.Contains(tk.body, "="):
			stack = append(stack, w)
		case blockOpeners[w]:
			stack = append(stack, w)
		case strings.HasPrefix(w, "end"):
			if len(stack) == genDepth {
				genPrefill(t, file, gen.String())
				gen.Reset()
				genDepth, genOn = -1, false
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	joined := strings.Join(strs, "")
	for _, g := range sstiStrings {
		if strings.Contains(joined, g) {
			t.add(crit, file, "sandbox-escape gadget %q assembled from string literals", g)
		}
	}
	if len(escaped) > 0 {
		checkText(t, file+" (escaped string literals)", strings.Join(escaped, "\n"), true)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func ssti(t *target, file string, e exprInfo) {
	for _, w := range append(append([]string{}, e.idents...), e.attrs...) {
		switch {
		case sstiCrit[w]:
			t.add(crit, file, "Jinja sandbox escape: %q referenced", w)
		case sstiHigh[w]:
			t.add(high, file, "Jinja global %q, a common sandbox-escape entry point", w)
		}
	}
	if contains(e.filters, "attr") {
		t.add(high, file, "attr filter: attribute access by computed name")
	}
}

func logicBomb(t *target, file, body string, e exprInfo) {
	hit := false
	for _, w := range append(append([]string{}, e.idents...), e.attrs...) {
		hit = hit || inspected[w]
	}
	if !hit {
		return
	}
	var odd []string
	for _, s := range e.strs {
		v := strings.TrimSpace(s)
		if len(v) < 4 || benignLit[strings.ToLower(v)] || strings.HasPrefix(v, "<") || strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") || strings.HasPrefix(v, "```") {
			continue
		}
		odd = append(odd, v)
	}
	if len(odd) > 0 {
		t.add(high, file, "condition tests message content for trigger literal(s) %q: {%% %s %%}", odd, clip(body, 160))
	}
}

func genPrefill(t *target, file, s string) {
	var words []string
	for _, w := range wordRe.FindAllString(specialRe.ReplaceAllString(s, " "), -1) {
		if !genBenign[strings.ToLower(w)] {
			words = append(words, w)
		}
	}
	if len(words) >= 4 {
		t.add(high, file, "generation prompt pre-fills the reply with text: %q", clip(strings.Join(strings.Fields(s), " "), 200))
	}
}

func scanTemplateJSON(t *target, name string, b []byte) {
	var v struct {
		ChatTemplate json.RawMessage `json:"chat_template"`
		AutoMap      map[string]any  `json:"auto_map"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.add(med, name, "not JSON: %v", err)
		return
	}
	for k, m := range v.AutoMap {
		ref := fmt.Sprint(m)
		sev := info
		if strings.Contains(ref, "--") {
			sev = med
		}
		t.add(sev, name, "auto_map %s -> %s: loading needs trust_remote_code", k, ref)
	}
	if len(v.ChatTemplate) == 0 || string(v.ChatTemplate) == "null" {
		return
	}
	var s string
	if json.Unmarshal(v.ChatTemplate, &s) == nil {
		t.addTemplate(name, s)
		return
	}
	var list []struct{ Name, Template string }
	if json.Unmarshal(v.ChatTemplate, &list) == nil {
		for _, x := range list {
			t.addTemplate(name+"#"+x.Name, x.Template)
		}
		return
	}
	t.add(med, name, "chat_template has an unexpected JSON type")
}
