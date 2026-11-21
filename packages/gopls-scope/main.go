// gopls-scope keeps gopls diagnostics to modules Claude Code is editing.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gopls-scope <gopls> [args...]")
		os.Exit(2)
	}
	err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gopls-scope:", err)
		os.Exit(1)
	}
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stderr = stderr
	toServer, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	fromServer, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	go func() {
		for sig := range signals {
			_ = cmd.Process.Signal(sig)
		}
	}()

	home, _ := os.UserHomeDir()
	s := newScope(goDirs(), home, stderr)
	served := make(chan error, 1)
	go func() { served <- pump(fromServer, stdout, s.fromServer) }()
	clientErr := pump(stdin, toServer, s.fromClient)
	_ = toServer.Close()
	serverErr := <-served
	waitErr := cmd.Wait()
	return errors.Join(clientErr, serverErr, waitErr)
}

func goDirs() []string {
	var dirs []string
	if out, err := exec.Command("go", "env", "GOROOT", "GOMODCACHE").Output(); err == nil {
		for line := range strings.Lines(string(out)) {
			if dir := strings.TrimSpace(line); filepath.IsAbs(dir) {
				dirs = append(dirs, filepath.Clean(dir))
			}
		}
	}
	if len(dirs) < 2 {
		if dir := os.Getenv("GOMODCACHE"); dir != "" {
			dirs = append(dirs, filepath.Clean(dir))
		} else if gopath := os.Getenv("GOPATH"); gopath != "" {
			dirs = append(dirs, filepath.Join(filepath.SplitList(gopath)[0], "pkg", "mod"))
		} else if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, "go", "pkg", "mod"))
		}
	}
	return dirs
}
