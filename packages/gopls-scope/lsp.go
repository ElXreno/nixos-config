package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func readMessage(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" && length < 0 {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("reading header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("bad Content-Length %q", value)
		}
		length = n
	}
	if length < 0 {
		return nil, errors.New("message without Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return body, nil
}

func writeMessage(w io.Writer, body []byte) error {
	msg := make([]byte, 0, len(body)+32)
	msg = fmt.Appendf(msg, "Content-Length: %d\r\n\r\n", len(body))
	msg = append(msg, body...)
	_, err := w.Write(msg)
	return err
}

func pump(r io.Reader, w io.Writer, handle func([]byte) [][]byte) error {
	br := bufio.NewReaderSize(r, 1<<16)
	for {
		body, err := readMessage(br)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, out := range handle(body) {
			if err := writeMessage(w, out); err != nil {
				return err
			}
		}
	}
}
