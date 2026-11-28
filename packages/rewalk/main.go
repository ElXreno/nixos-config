package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	standDir   = ".svc"
	socketName = "sock"
	paramsName = "params.json"
	toolName   = "orch-tool"
)

var tools = []string{"start", "diag", "fix", "scan", "replay"}

const readme = `# orchestrator

The orchestrator in this directory does not start. These tools are in bin/:

- bin/start tries to start the orchestrator and prints the failing stage with a few suspect subsystems.
- bin/diag <subsystem> <angle> prints a diagnostic report of one subsystem from one angle. The angles are: %s.
  A subsystem is healthy only when every angle reports no anomaly. Reports also name related subsystems.
- bin/fix <subsystem> <code> applies the fix for an anomaly that a report has shown.
- bin/scan runs every angle of every subsystem in one pass.
- bin/replay <subsystem> prints the stored reports of a subsystem.

The result of a check does not change unless a fix is applied to that subsystem.
The orchestrator starts once every faulty subsystem is fixed.
`

const throttleNote = `
A diagnostic run is slow: bin/diag runs once per turn, further runs in the same turn are refused.
`

type request struct {
	Args []string `json:"args"`
}

type response struct {
	Out  string `json:"out"`
	Code int    `json:"code"`
}

func gen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	seed := fs.Uint64("seed", 1, "scenario seed")
	dir := fs.String("dir", "", "workspace directory to create")
	tool := fs.String("tool", "", "path of the rewalk-tool binary, by default next to this program")
	subsystems := fs.Int("subsystems", 0, "number of subsystems")
	lines := fs.Int("lines", 0, "log lines per report")
	recue := fs.Float64("recue", -1, "probability that a report names a cleared subsystem")
	clear := fs.String("clear", "", "comma separated clearances needed per stage")
	budget := fs.Int("budget", 0, "diag calls allowed")
	throttle := fs.Bool("throttle", false, "allow one diag per tick")
	deadline := fs.Int("deadline", 0, "seconds after the first command when the tools stop answering")
	_ = fs.Parse(args)
	if *dir == "" {
		return errors.New("gen: -dir is required")
	}
	p := DefaultParams(*seed)
	if *subsystems > 0 {
		p.Subsystems = *subsystems
	}
	if *lines > 0 {
		p.Lines = *lines
	}
	if *recue >= 0 {
		p.Recue = *recue
	}
	if *budget > 0 {
		p.Budget = *budget
	}
	p.Throttle = *throttle
	p.Deadline = *deadline
	if *clear != "" {
		p.Clear = nil
		for part := range strings.SplitSeq(*clear, ",") {
			var n int
			if _, err := fmt.Sscanf(part, "%d", &n); err != nil {
				return fmt.Errorf("gen: -clear %q: %w", part, err)
			}
			p.Clear = append(p.Clear, n)
		}
	}
	need := len(p.Clear)
	for _, n := range p.Clear {
		need += n
	}
	if p.Subsystems < need+3 {
		return fmt.Errorf("gen: %d subsystems are too few for %d clearances", p.Subsystems, need)
	}
	if *tool == "" {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		*tool = filepath.Join(filepath.Dir(self), toolName)
	}
	binary, err := os.ReadFile(*tool)
	if err != nil {
		return err
	}
	for _, sub := range []string{"bin", standDir} {
		if err := os.MkdirAll(filepath.Join(*dir, sub), 0o755); err != nil {
			return err
		}
	}
	for _, name := range tools {
		if err := os.WriteFile(filepath.Join(*dir, "bin", name), binary, 0o755); err != nil {
			return err
		}
	}
	params, _ := json.MarshalIndent(p, "", "  ")
	if err := os.WriteFile(filepath.Join(*dir, standDir, paramsName), append(params, '\n'), 0o600); err != nil {
		return err
	}
	text := fmt.Sprintf(readme, strings.Join(p.Angles, ", "))
	if p.Throttle {
		text += throttleNote
	}
	return os.WriteFile(filepath.Join(*dir, "README.md"), []byte(text), 0o644)
}

func peerPid(conn net.Conn) int32 {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0
	}
	var pid int32
	_ = raw.Control(func(fd uintptr) {
		if cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED); err == nil {
			pid = cred.Pid
		}
	})
	return pid
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dir := fs.String("dir", "", "workspace directory made by gen")
	journalPath := fs.String("journal", "", "journal file, kept outside the workspace")
	_ = fs.Parse(args)
	if *dir == "" || *journalPath == "" {
		return errors.New("serve: -dir and -journal are required")
	}
	journalAbs, err := filepath.Abs(*journalPath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(*dir, standDir, paramsName))
	if err != nil {
		return err
	}
	var p Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(*dir, standDir, paramsName)); err != nil {
		return err
	}
	journal, err := os.OpenFile(journalAbs, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = journal.Close() }()
	if err := os.Chdir(filepath.Join(*dir, standDir)); err != nil {
		return err
	}
	_ = os.Remove(socketName)
	listener, err := net.Listen("unix", socketName)
	if err != nil {
		return err
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		_ = listener.Close()
	}()
	stand := NewStand(p, journal)
	var (
		mu      sync.Mutex
		tickPid int32
	)
	for {
		conn, err := listener.Accept()
		if err != nil {
			_ = os.Remove(socketName)
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go func() {
			defer func() { _ = conn.Close() }()
			var req request
			if json.NewDecoder(bufio.NewReader(conn)).Decode(&req) != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if len(req.Args) > 0 && req.Args[0] == "tick" {
				pid := peerPid(conn)
				if tickPid == 0 {
					tickPid = pid
				}
				if pid == 0 || pid != tickPid {
					req.Args = []string{"forged-tick"}
				}
			}
			out, code := stand.Run(req.Args)
			_ = json.NewEncoder(conn).Encode(response{out, code})
		}()
	}
}

func score(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	journalPath := fs.String("journal", "", "journal file written by serve")
	_ = fs.Parse(args)
	f, err := os.Open(*journalPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc, err := ScoreJournal(f)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(sc)
}

func approach(args []string) error {
	fs := flag.NewFlagSet("approach", flag.ExitOnError)
	commandsPath := fs.String("commands", "", "file with one agent command per line and --- at every compaction")
	_ = fs.Parse(args)
	f, err := os.Open(*commandsPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	a, err := ScoreApproach(f)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(a)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: rewalk gen|serve|score|approach [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "gen":
		err = gen(os.Args[2:])
	case "serve":
		err = serve(os.Args[2:])
	case "score":
		err = score(os.Args[2:])
	case "approach":
		err = approach(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rewalk:", err)
		os.Exit(1)
	}
}
