package env

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DirName   = "agent-memory"
	DBName    = "mem.db"
	CoreName  = "memory-core.md"
	AgentsDir = "agents"
)

type Env struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Getenv   func(string) string
	Now      func() time.Time
	Getwd    func() (string, error)
	Hostname func() (string, error)
	DBFlag   string
	AsFlag   string
}

func System() Env {
	var stdin io.Reader = os.Stdin
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		stdin = strings.NewReader("")
	}
	return Env{
		Stdin:    stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Getenv:   os.Getenv,
		Now:      time.Now,
		Getwd:    os.Getwd,
		Hostname: os.Hostname,
	}
}

func (e Env) Home() string {
	if h := e.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func (e Env) xdg(name, fallback string) string {
	if v := e.Getenv(name); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(e.Home(), fallback)
}

func (e Env) DataDir() string {
	return filepath.Join(e.xdg("XDG_DATA_HOME", ".local/share"), DirName)
}

func (e Env) Agent() string {
	return strings.ToLower(strings.TrimSpace(e.Getenv("MEM_AGENT")))
}

func (e Env) StateDir() string {
	dir := filepath.Join(e.xdg("XDG_STATE_HOME", ".local/state"), DirName)
	if a := e.Agent(); a != "" {
		return filepath.Join(dir, AgentsDir, a)
	}
	return dir
}

func (e Env) DBPath() string {
	if e.DBFlag != "" {
		return e.DBFlag
	}
	if v := e.Getenv("MEM_DB"); v != "" {
		return v
	}
	return filepath.Join(e.DataDir(), DBName)
}

func (e Env) CorePath() string {
	if v := e.Getenv("MEM_CORE"); v != "" {
		return v
	}
	if a := e.Agent(); a != "" {
		return filepath.Join(e.DataDir(), AgentsDir, a, CoreName)
	}
	return filepath.Join(e.Home(), ".claude", CoreName)
}

func (e Env) ProtectedDirs() []string {
	dirs := []string{filepath.Dir(e.DBPath()), e.CorePath()}
	for _, d := range strings.Split(e.Getenv("MEM_PROTECTED_DIRS"), ":") {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func (e Env) Host() string {
	if e.Hostname == nil {
		return ""
	}
	h, err := e.Hostname()
	if err != nil {
		return ""
	}
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	return strings.ToLower(h)
}
