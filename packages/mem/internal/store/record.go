package store

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Kind string

const (
	Rule     Kind = "rule"
	Fact     Kind = "fact"
	State    Kind = "state"
	Incident Kind = "incident"
	Howto    Kind = "howto"
	Log      Kind = "log"
)

var Kinds = []Kind{Rule, State, Fact, Incident, Howto, Log}

type Status string

const (
	Current    Status = "current"
	Superseded Status = "superseded"
	Expired    Status = "expired"
	Retired    Status = "retired"
)

var Statuses = []Status{Current, Superseded, Expired, Retired}

type Rel string

const (
	Supersedes  Rel = "supersedes"
	Relates     Rel = "relates"
	DerivedFrom Rel = "derived_from"
	Unresolved  Rel = "unresolved"
)

var Rels = []Rel{Supersedes, Relates, DerivedFrom, Unresolved}

const (
	GlobalScope     = "global"
	DefaultStateTTL = 14
	MaxKeyLen       = 200
)

type Trigger struct {
	Tool  string `json:"tool"`
	Regex string `json:"regex"`
}

type Record struct {
	Key          string    `json:"key"`
	Kind         Kind      `json:"kind"`
	Scopes       []string  `json:"scopes"`
	Core         bool      `json:"core,omitempty"`
	Title        string    `json:"title,omitempty"`
	Body         string    `json:"body"`
	Status       Status    `json:"status"`
	SupersededBy string    `json:"superseded_by,omitempty"`
	ValidFrom    time.Time `json:"valid_from,omitzero"`
	ValidTo      time.Time `json:"valid_to,omitzero"`
	TTLDays      int       `json:"ttl_days,omitempty"`
	VerifyCmd    string    `json:"verify_cmd,omitempty"`
	VerifiedAt   time.Time `json:"verified_at,omitzero"`
	Triggers     []Trigger `json:"triggers,omitempty"`
	Source       string    `json:"source,omitempty"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at,omitzero"`
	UpdatedAt    time.Time `json:"updated_at,omitzero"`
}

type Version struct {
	Record
	ChangedAt time.Time `json:"changed_at"`
	Reason    string    `json:"reason"`
}

type Link struct {
	Src  string `json:"src"`
	Dst  string `json:"dst"`
	Rel  Rel    `json:"rel"`
	Note string `json:"note,omitempty"`
}

func (r Record) AsOf() time.Time {
	if r.VerifiedAt.After(r.UpdatedAt) {
		return r.VerifiedAt
	}
	return r.UpdatedAt
}

func (r Record) ExpiresAt() time.Time {
	ttl := r.TTLDays
	if ttl <= 0 {
		ttl = DefaultStateTTL
	}
	return r.AsOf().Add(time.Duration(ttl) * 24 * time.Hour)
}

func (r Record) ExpiredSince() time.Time {
	if !r.ValidTo.IsZero() {
		return r.ValidTo
	}
	return r.UpdatedAt
}

func (r Record) HasScope(scope string) bool {
	return slices.Contains(r.Scopes, scope)
}

var ErrInvalid = errors.New("invalid")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

var (
	keyRe         = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:#-]*$`)
	scopeNameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_.+@-]*$`)
	projectNameRe = regexp.MustCompile(`^[a-z0-9~][a-z0-9_.+@~-]*(/[a-z0-9_.+@~-]+)*$`)
)

func validProjectName(name string) bool {
	if !projectNameRe.MatchString(name) {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func ValidKey(key string) error {
	if len(key) > MaxKeyLen || !keyRe.MatchString(key) {
		return invalid("key %q must match [a-z0-9][a-z0-9_.:#-]* and be at most %d bytes", key, MaxKeyLen)
	}
	return nil
}

func ParseKind(s string) (Kind, error) {
	k := Kind(strings.ToLower(strings.TrimSpace(s)))
	if !slices.Contains(Kinds, k) {
		return "", invalid("kind %q is not one of rule, fact, state, incident, howto, log", s)
	}
	return k, nil
}

func ParseStatus(s string) (Status, error) {
	st := Status(strings.ToLower(strings.TrimSpace(s)))
	if !slices.Contains(Statuses, st) {
		return "", invalid("status %q is not one of current, superseded, expired, retired", s)
	}
	return st, nil
}

func ParseRel(s string) (Rel, error) {
	r := Rel(strings.ToLower(strings.TrimSpace(s)))
	if !slices.Contains(Rels, r) {
		return "", invalid("link relation %q is not one of supersedes, relates, derived_from, unresolved", s)
	}
	return r, nil
}

func NormalizeScope(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == GlobalScope {
		return s, nil
	}
	prefix, name, ok := strings.Cut(s, ":")
	valid := ok && (prefix == "project" && validProjectName(name) || (prefix == "host" || prefix == "topic" || prefix == "agent") && scopeNameRe.MatchString(name))
	if !valid {
		return "", invalid("scope %q is not global, project:<name>, host:<name>, topic:<id> or agent:<name>", s)
	}
	return s, nil
}

func NormalizeScopes(in []string) ([]string, error) {
	var out []string
	for _, s := range in {
		n, err := NormalizeScope(s)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	if n := len(slices.DeleteFunc(slices.Clone(out), func(s string) bool { return !isTopicScope(s) })); n > 1 {
		return nil, invalid("a record belongs to one topic, got %d topic scopes", n)
	}
	if n := len(slices.DeleteFunc(slices.Clone(out), func(s string) bool { return !isAgentScope(s) })); n > 1 {
		return nil, invalid("a record belongs to one agent, got %d agent scopes", n)
	}
	slices.Sort(out)
	return out, nil
}

func ProjectScope(name string) string {
	return "project:" + strings.ToLower(name)
}

func HostScope(name string) string {
	if i := strings.IndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return "host:" + strings.ToLower(name)
}

func ParseTrigger(s string) (Trigger, error) {
	tool, re, ok := strings.Cut(s, ":")
	if !ok || tool == "" || re == "" {
		return Trigger{}, invalid("trigger %q is not TOOL:REGEX", s)
	}
	t := Trigger{Tool: tool, Regex: re}
	return t, t.validate()
}

func (t Trigger) validate() error {
	if _, err := regexp.Compile(`^(?:` + t.Tool + `)$`); err != nil {
		return invalid("trigger tool %q: %v", t.Tool, err)
	}
	if _, err := regexp.Compile(t.Regex); err != nil {
		return invalid("trigger regex %q: %v", t.Regex, err)
	}
	return nil
}

func (t Trigger) String() string {
	return t.Tool + ":" + t.Regex
}

func normalizeTriggers(in []Trigger) ([]Trigger, error) {
	var out []Trigger
	for _, t := range in {
		if t.Tool == "" || t.Regex == "" {
			return nil, invalid("trigger %q is not TOOL:REGEX", t.String())
		}
		if err := t.validate(); err != nil {
			return nil, err
		}
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b Trigger) int {
		if c := strings.Compare(a.Tool, b.Tool); c != 0 {
			return c
		}
		return strings.Compare(a.Regex, b.Regex)
	})
	return out, nil
}

func (r *Record) normalize() error {
	if err := ValidKey(r.Key); err != nil {
		return err
	}
	k, err := ParseKind(string(r.Kind))
	if err != nil {
		return err
	}
	r.Kind = k
	if r.Scopes, err = NormalizeScopes(r.Scopes); err != nil {
		return err
	}
	if len(r.Scopes) == 0 || len(r.Scopes) == 1 && isAgentScope(r.Scopes[0]) {
		return invalid("record %s needs at least one scope", r.Key)
	}
	if r.Triggers, err = normalizeTriggers(r.Triggers); err != nil {
		return err
	}
	r.Title = strings.TrimSpace(r.Title)
	r.Body = cleanBody(r.Body)
	if r.Body == "" {
		return invalid("record %s has an empty body", r.Key)
	}
	if r.TTLDays < 0 {
		return invalid("ttl must be positive")
	}
	if r.Kind != State && r.TTLDays != 0 {
		return invalid("ttl applies to state records only")
	}
	if r.Kind == State && r.TTLDays == 0 {
		r.TTLDays = DefaultStateTTL
	}
	if r.Status == "" {
		r.Status = Current
	}
	if _, err := ParseStatus(string(r.Status)); err != nil {
		return err
	}
	return nil
}

func cleanBody(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, " \t\n")
	return strings.TrimLeft(s, "\n")
}

func sameContent(a, b Record) bool {
	return a.Kind == b.Kind && slices.Equal(a.Scopes, b.Scopes) && a.Core == b.Core && a.Title == b.Title &&
		a.Body == b.Body && a.TTLDays == b.TTLDays && a.VerifyCmd == b.VerifyCmd && a.Source == b.Source &&
		slices.Equal(a.Triggers, b.Triggers)
}

func sameClaim(a, b Record) bool {
	a.Triggers, b.Triggers = nil, nil
	return sameContent(a, b)
}

func (r Record) Validate() error {
	return r.normalize()
}
