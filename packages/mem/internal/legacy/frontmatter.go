package legacy

import (
	"strconv"
	"strings"
	"time"
)

var bom = string(rune(0xfeff))

type frontmatter struct {
	top    map[string]string
	nested map[string]map[string]string
}

func (f frontmatter) get(key string) string {
	if v, ok := f.top[key]; ok && v != "" {
		return v
	}
	for _, parent := range []string{"metadata"} {
		if v := f.nested[parent][key]; v != "" {
			return v
		}
	}
	return ""
}

func (f frontmatter) Name() string        { return f.get("name") }
func (f frontmatter) Description() string { return f.get("description") }
func (f frontmatter) Type() string        { return strings.ToLower(f.get("type")) }

func (f frontmatter) Modified() time.Time {
	v := f.get("modified")
	if v == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC().Truncate(time.Second)
		}
	}
	return time.Time{}
}

func splitFrontmatter(src string) (frontmatter, string, bool) {
	src = strings.TrimPrefix(src, bom)
	src = strings.ReplaceAll(src, "\r\n", "\n")
	fm := frontmatter{top: map[string]string{}, nested: map[string]map[string]string{}}
	if !strings.HasPrefix(src, "---\n") {
		return fm, src, false
	}
	rest := src[len("---\n"):]
	var block string
	switch {
	case strings.HasPrefix(rest, "---\n") || rest == "---":
		block, rest = "", strings.TrimPrefix(strings.TrimPrefix(rest, "---"), "\n")
	default:
		end := strings.Index(rest, "\n---\n")
		if end < 0 {
			if !strings.HasSuffix(rest, "\n---") {
				return fm, src, false
			}
			end = len(rest) - len("\n---")
			block, rest = rest[:end], ""
		} else {
			block, rest = rest[:end], rest[end+len("\n---\n"):]
		}
	}
	parseYAML(block, &fm)
	return fm, rest, true
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func parseYAML(block string, fm *frontmatter) {
	lines := strings.Split(block, "\n")
	parent := ""
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := indentOf(line)
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok || strings.ContainsAny(key, " \t\"'") {
			continue
		}
		val = strings.TrimSpace(val)
		if indent == 0 {
			parent = ""
		}
		next := i + 1
		var cont []string
		for next < len(lines) {
			nl := strings.TrimRight(lines[next], " \t")
			if strings.TrimSpace(nl) == "" {
				cont = append(cont, "")
				next++
				continue
			}
			if indentOf(nl) <= indent {
				break
			}
			if val == "" && !isBlockIndicator(val) {
				break
			}
			cont = append(cont, nl)
			next++
		}
		for len(cont) > 0 && cont[len(cont)-1] == "" {
			cont = cont[:len(cont)-1]
		}
		if val == "" && indent == 0 {
			parent = key
			if fm.nested[key] == nil {
				fm.nested[key] = map[string]string{}
			}
			continue
		}
		i = next - 1
		v := scalar(val, cont)
		if indent > 0 && parent != "" {
			fm.nested[parent][key] = v
		} else {
			fm.top[key] = v
		}
	}
}

func isBlockIndicator(v string) bool {
	return v != "" && (v[0] == '|' || v[0] == '>')
}

func scalar(val string, cont []string) string {
	switch {
	case isBlockIndicator(val):
		var parts []string
		for _, c := range cont {
			parts = append(parts, strings.TrimSpace(c))
		}
		if val[0] == '|' {
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
		return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	case strings.HasPrefix(val, `"`):
		full := strings.Join(append([]string{val}, trimAll(cont)...), " ")
		if s, err := strconv.Unquote(full); err == nil {
			return s
		}
		s := strings.TrimSuffix(strings.TrimPrefix(full, `"`), `"`)
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\n`, "\n", `\t`, "\t").Replace(s)
	case strings.HasPrefix(val, `'`):
		full := strings.Join(append([]string{val}, trimAll(cont)...), " ")
		s := strings.TrimSuffix(strings.TrimPrefix(full, `'`), `'`)
		return strings.ReplaceAll(s, `''`, `'`)
	}
	return strings.Join(append([]string{val}, trimAll(cont)...), " ")
}

func trimAll(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
