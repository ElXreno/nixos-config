package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"agent-guard/internal/checks"
	"agent-guard/internal/edit"
	"agent-guard/internal/evidence"
	"agent-guard/internal/harness"
	"agent-guard/internal/power"
	"agent-guard/internal/treesitter"
)

type Config struct {
	Home           string                `json:"home"`
	Harness        string                `json:"harness"`
	Journal        string                `json:"journal"`
	Transcripts    string                `json:"transcripts"`
	ShellReadRoots []string              `json:"shellReadRoots"`
	CreateRoots    []string              `json:"createRoots"`
	ReadOnlyRoots  []checks.ReadOnlyRoot `json:"readOnlyRoots"`
	DeniedWrites   []checks.DeniedWrite  `json:"deniedWrites"`
	ScratchBase    string                `json:"scratchBase"`
	ReposRoot      string                `json:"reposRoot"`
	AvoidRoots     []string              `json:"avoidRoots"`
	MemoryPaths    []string              `json:"memoryPaths"`
	ProtectedRepos []string              `json:"protectedRepos"`
	AgentStores    []string              `json:"agentStores"`
	Power          Power                 `json:"power"`
	EvidenceTools  []string              `json:"evidenceTools"`
	Grammars       treesitter.Grammars   `json:"grammars"`

	Spec harness.Spec `json:"-"`
}

type Power struct {
	Enable        bool     `json:"enable"`
	HeavyCommands []string `json:"heavyCommands"`
}

func Load(path string, getenv func(string) string) (*Config, error) {
	cfg := &Config{}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(cfg); err != nil {
			return nil, err
		}
		if cfg.Harness == "" {
			return nil, errors.New("harness is required in a config file")
		}
	}
	spec, err := harness.ByID(cfg.Harness)
	if err != nil {
		return nil, err
	}
	cfg.Spec = spec
	if cfg.Home == "" {
		cfg.Home = getenv("HOME")
	}
	if getenv("MEM_AGENT") != "" {
		cfg.AgentStores = nil
	}
	return cfg, nil
}

func (c *Config) Check() error {
	var errs []error
	absolute := func(what, p string) {
		if !filepath.IsAbs(p) {
			errs = append(errs, fmt.Errorf("%s %q is not an absolute path", what, p))
		}
	}
	absolute("scratchBase", c.ScratchBase)
	absolute("journal", c.Journal)
	if c.ReposRoot != "" {
		absolute("reposRoot", c.ReposRoot)
	}
	if c.Transcripts != "" {
		absolute("transcripts", c.Transcripts)
	}
	for _, p := range c.ShellReadRoots {
		absolute("shellReadRoots entry", p)
	}
	for _, p := range c.CreateRoots {
		absolute("createRoots entry", p)
	}
	for _, p := range c.AvoidRoots {
		absolute("avoidRoots entry", p)
	}
	for _, r := range c.ReadOnlyRoots {
		absolute("readOnlyRoots path", r.Path)
		if r.Reason == "" {
			errs = append(errs, fmt.Errorf("readOnlyRoots entry %q has no reason", r.Path))
		}
	}
	for _, d := range c.DeniedWrites {
		absolute("deniedWrites path", d.Path)
		if d.Reason == "" {
			errs = append(errs, fmt.Errorf("deniedWrites entry %q has no reason", d.Path))
		}
	}
	for _, p := range c.AgentStores {
		absolute("agentStores entry", p)
	}
	for _, p := range c.ProtectedRepos {
		if err := checks.ValidRepoPattern(p); err != nil {
			errs = append(errs, fmt.Errorf("protectedRepos entry %q: %w", p, err))
		}
	}
	if _, err := power.Parse(c.Power.HeavyCommands); err != nil {
		errs = append(errs, fmt.Errorf("power: %w", err))
	}
	for _, p := range c.EvidenceTools {
		if err := evidence.ValidPattern(p); err != nil {
			errs = append(errs, fmt.Errorf("evidenceTools entry %q: %w", p, err))
		}
	}
	for name := range c.Grammars {
		if _, err := c.Grammars.Load(name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Config) PowerState() string {
	return c.stateDir("power")
}

func (c *Config) EvidenceState() string {
	return c.stateDir("evidence")
}

func (c *Config) stateDir(name string) string {
	if c.Journal == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.Journal), name)
}

func (c *Config) Checks() *checks.Settings {
	return &checks.Settings{
		Home:           c.Home,
		ShellReadRoots: c.ShellReadRoots,
		CreateRoots:    c.CreateRoots,
		ReadOnlyRoots:  c.ReadOnlyRoots,
		DeniedWrites:   c.DeniedWrites,
		ScratchBase:    c.ScratchBase,
		ReposRoot:      c.ReposRoot,
		AvoidRoots:     c.AvoidRoots,
		ProtectedRepos: c.ProtectedRepos,
		AgentStores:    c.AgentStores,
		Spec:           c.Spec,
	}
}

func (c *Config) EditOptions() edit.Options {
	return edit.Options{Grammars: c.Grammars, Vocab: c.Spec.Vocab, Scratch: c.ScratchBase}
}

func (c *Config) Memory() func(path, cwd string) bool {
	if len(c.MemoryPaths) == 0 {
		return nil
	}
	return edit.MemoryMatcher(c.MemoryPaths)
}
