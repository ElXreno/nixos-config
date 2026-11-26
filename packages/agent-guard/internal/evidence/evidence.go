package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"agent-guard/internal/statefile"
)

const (
	recordLimit = 20000
	shownKey    = "limitations_shown_earlier"
	sameAsKey   = "same_as"

	detachedNote   = "shown below the JSON"
	detachedHeader = "normalized_result:"
)

var dropEmpty = map[string]bool{
	"environment":           true,
	"relationship_coverage": true,
	"locations":             true,
	"evidence_links":        true,
}

var envelope = map[string]bool{
	"subject":          true,
	"analysis_profile": true,
}

var envelopeGroup = map[string]bool{
	"evidence_format":  true,
	"subject":          true,
	"provider":         true,
	"analysis_profile": true,
	"predicate_type":   true,
	"confidence":       true,
	"authority":        true,
}

var hexNumber = regexp.MustCompile(`0x[0-9A-Fa-f]+`)

type Memory struct {
	Dir string
	Now func() time.Time
}

type state struct {
	Seen map[string]string `json:"seen"`
}

func (m Memory) now() time.Time {
	if m.Now == nil {
		return time.Now()
	}
	return m.Now()
}

func Matches(patterns []string, tool string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, tool); ok {
			return true
		}
	}
	return false
}

func ValidPattern(p string) error {
	if p == "" {
		return errors.New("empty pattern")
	}
	_, err := path.Match(p, "")
	return err
}

func (m Memory) Compact(key, text string) (string, bool, error) {
	body := bytes.TrimSpace([]byte(text))
	if len(body) == 0 || (body[0] != '{' && body[0] != '[') || !json.Valid(body) {
		return text, false, nil
	}
	f, err := statefile.Lock(m.Dir, key)
	if err != nil {
		return text, false, err
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(f)
	if err != nil {
		return text, false, err
	}
	var st state
	created := len(raw) == 0 || json.Unmarshal(raw, &st) != nil
	if st.Seen == nil {
		st.Seen = map[string]string{}
	}
	c := compactor{seen: st.Seen, fresh: map[string]string{}, learn: true}
	out, err := c.value(body, "")
	if err == nil && len(out) > recordLimit {
		c = compactor{seen: st.Seen, fresh: map[string]string{}}
		out, err = c.value(body, "")
	}
	if err != nil {
		return text, false, err
	}
	if len(c.fresh) > 0 || created {
		maps.Copy(st.Seen, c.fresh)
		data, err := json.Marshal(st)
		if err != nil {
			return text, false, err
		}
		if err := statefile.Replace(f, append(data, '\n')); err != nil {
			return text, false, err
		}
		if created {
			statefile.Prune(m.Dir, f.Name(), m.now())
		}
	}
	if detached, ok := detach(out); ok {
		return detached, true, nil
	}
	if len(out) >= len(text) {
		return text, false, nil
	}
	return string(out), true, nil
}

func detach(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return "", false
	}
	members, err := object(raw)
	if err != nil {
		return "", false
	}
	record, at := false, -1
	for i, m := range members {
		switch m.key {
		case "evidence_id":
			record = true
		case "normalized_result":
			at = i
		}
	}
	if !record || at < 0 {
		return "", false
	}
	var text string
	if json.Unmarshal(members[at].val, &text) != nil || !strings.Contains(text, "\n") {
		return "", false
	}
	members[at].val = quote(detachedNote)
	return string(encodeObject(members)) + "\n\n" + detachedHeader + "\n" + text, true
}

func (m Memory) Forget(prefix string) error {
	if m.Dir == "" || prefix == "" {
		return nil
	}
	entries, err := os.ReadDir(m.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	whole := statefile.Name(prefix)
	stem := strings.TrimSuffix(whole, ".json") + "-"
	var errs []error
	for _, e := range entries {
		if n := e.Name(); n == whole || (strings.HasPrefix(n, stem) && strings.HasSuffix(n, ".json")) {
			if err := os.Remove(filepath.Join(m.Dir, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

type compactor struct {
	seen  map[string]string
	fresh map[string]string
	learn bool
}

func (c *compactor) known(k string) (string, bool) {
	if v, ok := c.seen[k]; ok {
		return v, true
	}
	v, ok := c.fresh[k]
	return v, ok
}

func (c *compactor) remember(k, evidence string) {
	if c.learn {
		c.fresh[k] = evidence
	}
}

type member struct {
	key string
	val json.RawMessage
}

func (c *compactor) value(raw json.RawMessage, evidence string) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return raw, nil
	}
	switch raw[0] {
	case '{':
		members, err := object(raw)
		if err != nil {
			return nil, err
		}
		return c.object(members, evidence)
	case '[':
		var elems []json.RawMessage
		if err := json.Unmarshal(raw, &elems); err != nil {
			return nil, err
		}
		for i, e := range elems {
			v, err := c.value(e, evidence)
			if err != nil {
				return nil, err
			}
			elems[i] = v
		}
		return encodeArray(elems), nil
	}
	return raw, nil
}

func (c *compactor) object(members []member, outer string) (json.RawMessage, error) {
	evidence := ""
	var normalized json.RawMessage
	for _, m := range members {
		switch m.key {
		case "evidence_id":
			_ = json.Unmarshal(m.val, &evidence)
		case "normalized_result":
			normalized = m.val
		}
	}
	record := evidence != ""
	if !record {
		evidence = outer
	}
	sameAs, group := "", envelopeMembers(members)
	if record && len(group) > 1 {
		k := "envelope:" + groupDigest(group)
		if first, ok := c.known(k); ok {
			sameAs = first
		} else {
			c.remember(k, evidence)
		}
	}
	out := make([]member, 0, len(members)+1)
	placed := false
	for _, m := range members {
		switch {
		case sameAs != "" && envelopeGroup[m.key]:
			if !placed {
				out = append(out, member{sameAsKey, sameAsValue(sameAs, group)})
				placed = true
			}
			continue
		case m.key == "limitations":
			kept, shown, ok := c.limitations(m.val, evidence)
			if !ok || len(kept)+shown == 0 {
				break
			}
			if len(kept) > 0 {
				out = append(out, member{m.key, encodeArray(merge(kept))})
			}
			if shown > 0 {
				n, _ := json.Marshal(shown)
				out = append(out, member{shownKey, n})
			}
			continue
		case record && envelope[m.key] && m.val[0] == '{':
			k := m.key + ":" + digest(m.val)
			if first, ok := c.known(k); ok {
				out = append(out, member{m.key, quote("same as in " + first)})
				continue
			}
			c.remember(k, evidence)
		case record && dropEmpty[m.key] && empty(m.val):
			continue
		case record && m.key == "raw_result" && normalized != nil && !empty(m.val) && same(m.val, normalized):
			out = append(out, member{m.key, quote("same as normalized_result")})
			continue
		}
		v, err := c.value(m.val, evidence)
		if err != nil {
			return nil, err
		}
		out = append(out, member{m.key, v})
	}
	return encodeObject(out), nil
}

func (c *compactor) limitations(raw json.RawMessage, evidence string) ([]json.RawMessage, int, bool) {
	var elems []json.RawMessage
	if json.Unmarshal(raw, &elems) != nil || elems == nil {
		return nil, 0, false
	}
	kept := make([]json.RawMessage, 0, len(elems))
	shown := 0
	for _, e := range elems {
		var s string
		if json.Unmarshal(e, &s) != nil {
			return nil, 0, false
		}
		k := "limitation:" + digest([]byte(s))
		if _, ok := c.known(k); ok {
			shown++
			continue
		}
		c.remember(k, evidence)
		kept = append(kept, e)
	}
	return kept, shown, true
}

func object(raw json.RawMessage) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var members []member
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k, ok := t.(string)
		if !ok {
			return nil, errors.New("object key is not a string")
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		members = append(members, member{k, v})
	}
	return members, nil
}

func encodeObject(members []member) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(quote(m.key))
		b.WriteByte(':')
		b.Write(m.val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

func encodeArray(elems []json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, e := range elems {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(e)
	}
	b.WriteByte(']')
	return b.Bytes()
}

func quote(s string) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n")
}

func digest(b []byte) string {
	var compact bytes.Buffer
	if json.Compact(&compact, b) == nil {
		b = compact.Bytes()
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

func empty(raw json.RawMessage) bool {
	s := string(bytes.TrimSpace(raw))
	return s == "null" || s == "[]" || s == "{}"
}

func same(a, b json.RawMessage) bool {
	decode := func(raw json.RawMessage) (any, bool) {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var v any
		return v, dec.Decode(&v) == nil
	}
	x, ok := decode(a)
	if !ok {
		return false
	}
	y, ok := decode(b)
	return ok && reflect.DeepEqual(x, y)
}

func envelopeMembers(members []member) []member {
	var group []member
	for _, m := range members {
		if envelopeGroup[m.key] {
			group = append(group, m)
		}
	}
	return group
}

func groupDigest(group []member) string {
	var b bytes.Buffer
	for _, m := range group {
		b.WriteString(m.key)
		b.WriteByte('=')
		b.WriteString(digest(m.val))
		b.WriteByte('\n')
	}
	return digest(b.Bytes())
}

func sameAsValue(evidence string, group []member) json.RawMessage {
	fields := make([]string, len(group))
	for i, m := range group {
		fields[i] = m.key
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(struct {
		Evidence string   `json:"evidence_id"`
		Fields   []string `json:"fields"`
	}{evidence, fields})
	return bytes.TrimRight(b.Bytes(), "\n")
}

func merge(elems []json.RawMessage) []json.RawMessage {
	texts := make([]string, len(elems))
	forms := make([]string, len(elems))
	byForm := map[string][]int{}
	for i, e := range elems {
		_ = json.Unmarshal(e, &texts[i])
		forms[i] = hexNumber.ReplaceAllString(texts[i], "\x00")
		byForm[forms[i]] = append(byForm[forms[i]], i)
	}
	out := make([]json.RawMessage, 0, len(elems))
	done := map[string]bool{}
	for i, e := range elems {
		if done[forms[i]] {
			continue
		}
		merged, ok := mergeForm(forms[i], texts, byForm[forms[i]])
		if !ok {
			out = append(out, e)
			continue
		}
		done[forms[i]] = true
		out = append(out, quote(merged))
	}
	return out
}

func mergeForm(form string, texts []string, members []int) (string, bool) {
	if len(members) < 2 {
		return "", false
	}
	values := make([][]string, len(members))
	for i, m := range members {
		values[i] = hexNumber.FindAllString(texts[m], -1)
	}
	varying := -1
	for pos := range values[0] {
		for _, v := range values[1:] {
			if v[pos] != values[0][pos] {
				if varying != -1 && varying != pos {
					return "", false
				}
				varying = pos
			}
		}
	}
	if varying == -1 {
		return "", false
	}
	parts := strings.Split(form, "\x00")
	var b strings.Builder
	for pos, part := range parts {
		b.WriteString(part)
		if pos == len(parts)-1 {
			break
		}
		if pos != varying {
			b.WriteString(values[0][pos])
			continue
		}
		var seen []string
		for _, v := range values {
			if !slices.Contains(seen, v[pos]) {
				seen = append(seen, v[pos])
			}
		}
		b.WriteByte('[')
		b.WriteString(strings.Join(seen, ", "))
		b.WriteByte(']')
	}
	return b.String(), true
}
