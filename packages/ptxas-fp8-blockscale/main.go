package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	versionMarker = "ptxas-fp8-blockscale: FP8 MMA with FP32 accumulation is emitted block-scaled on sm_120"
	activeVar     = "PTXAS_FP8_ACTIVE"
)

func main() {
	if os.Getenv(activeVar) != "" {
		fmt.Fprintln(os.Stderr, "ptxas-fp8-blockscale: the ptxas it runs is this wrapper again, check PTXAS_FP8_REAL")
		os.Exit(127)
	}
	real, err := resolveReal()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptxas-fp8-blockscale:", err)
		os.Exit(127)
	}
	args := os.Args[1:]
	enabled := os.Getenv("PTXAS_FP8_BLOCKSCALE") != "0"
	var temps []string
	if enabled {
		args, temps = rewriteInputs(args)
	}
	code := run(real, args)
	for _, t := range temps {
		_ = os.Remove(t)
	}
	if enabled && code == 0 && versionQuery(args) {
		fmt.Println(versionMarker)
	}
	os.Exit(code)
}

func resolveReal() (string, error) {
	if p := os.Getenv("PTXAS_FP8_REAL"); p != "" {
		return p, nil
	}
	if maps, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", os.Getppid())); err == nil {
		if p := tritonPtxas(string(maps)); p != "" {
			return p, nil
		}
	}
	if p := os.Getenv("PTXAS_FP8_FALLBACK"); p != "" {
		return p, nil
	}
	return "", errors.New("no ptxas to run: set PTXAS_FP8_REAL or PTXAS_FP8_FALLBACK")
}

func tritonPtxas(maps string) string {
	for line := range strings.SplitSeq(maps, "\n") {
		i := strings.Index(line, "/triton/_C/libtriton")
		start := strings.IndexByte(line, '/')
		if i < 0 || start < 0 || start > i {
			continue
		}
		p := filepath.Join(line[start:i+len("/triton")], "backends", "nvidia", "bin", "ptxas-blackwell")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func rewriteInputs(args []string) ([]string, []string) {
	out := append([]string(nil), args...)
	var temps []string
	for i, a := range out {
		if !strings.HasSuffix(a, ".ptx") {
			continue
		}
		src, err := os.ReadFile(a)
		if err != nil {
			continue
		}
		res := Rewrite(string(src))
		if res.Skip != "" {
			logf("skip %s: %s", a, res.Skip)
		}
		if res.Count == 0 {
			continue
		}
		name, err := writeTemp(res.PTX)
		if err != nil {
			logf("keep %s: %v", a, err)
			continue
		}
		out[i] = name
		temps = append(temps, name)
		logf("rewrote %d in %s", res.Count, a)
	}
	return out, temps
}

func writeTemp(ptx string) (string, error) {
	f, err := os.CreateTemp("", "ptxas-fp8-*.ptx")
	if err != nil {
		return "", err
	}
	_, werr := f.WriteString(ptx)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func run(real string, args []string) int {
	cmd := exec.Command(real, args...)
	cmd.Env = append(os.Environ(), activeVar+"=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		return exit.ExitCode()
	default:
		fmt.Fprintln(os.Stderr, "ptxas-fp8-blockscale:", err)
		return 127
	}
}

func versionQuery(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-V" {
			return true
		}
	}
	return false
}

func logf(format string, a ...any) {
	p := os.Getenv("PTXAS_FP8_LOG")
	if p == "" {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(f, format+"\n", a...)
	_ = f.Close()
}
