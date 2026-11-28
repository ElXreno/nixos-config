package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type request struct {
	Args []string `json:"args"`
}

type response struct {
	Out  string `json:"out"`
	Code int    `json:"code"`
}

func run() int {
	tool := filepath.Base(os.Args[0])
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, tool+":", err)
		return 70
	}
	if err := os.Chdir(filepath.Join(filepath.Dir(self), "..", ".svc")); err != nil {
		fmt.Fprintln(os.Stderr, tool+": the service is not reachable")
		return 70
	}
	conn, err := net.Dial("unix", "sock")
	if err != nil {
		fmt.Fprintln(os.Stderr, tool+": the service is not reachable")
		return 70
	}
	defer func() { _ = conn.Close() }()
	if err := json.NewEncoder(conn).Encode(request{append([]string{tool}, os.Args[1:]...)}); err != nil {
		fmt.Fprintln(os.Stderr, tool+":", err)
		return 70
	}
	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		fmt.Fprintln(os.Stderr, tool+":", err)
		return 70
	}
	_, _ = os.Stdout.WriteString(resp.Out)
	return resp.Code
}

func main() {
	os.Exit(run())
}
