package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type replacement struct {
	Old string `json:"old"`
	New string `json:"new"`
}

type modulePatch struct {
	Module  string        `json:"module"`
	Replace []replacement `json:"replace"`
}

func loadPatches(sets []string) ([]modulePatch, error) {
	var out []modulePatch
	at := map[string]int{}
	for _, set := range sets {
		raw, err := os.ReadFile(set)
		if err != nil {
			return nil, err
		}
		var ps []modulePatch
		if err := json.Unmarshal(raw, &ps); err != nil {
			return nil, fmt.Errorf("%s: %v", set, err)
		}
		for _, p := range ps {
			if len(p.Replace) == 0 {
				return nil, fmt.Errorf("%s: %s has no replacements", set, p.Module)
			}
			if i, seen := at[p.Module]; seen {
				out[i].Replace = append(out[i].Replace, p.Replace...)
				continue
			}
			at[p.Module] = len(out)
			out = append(out, p)
		}
	}
	return out, nil
}

func patchFile(file []byte, patches []modulePatch) ([]byte, []string, error) {
	b, err := locateBun(file)
	if err != nil {
		return nil, nil, err
	}
	g, err := parseGraph(b.payload)
	if err != nil {
		return nil, nil, err
	}
	payload, changes, err := g.patch(patches)
	if err != nil {
		return nil, nil, err
	}
	out, delta, err := b.rewrite(payload)
	if err != nil {
		return nil, nil, err
	}
	if err := verify(b, g, out, delta, changes); err != nil {
		return nil, nil, fmt.Errorf("self-check failed: %v", err)
	}
	var report []string
	for _, c := range changes {
		how := fmt.Sprintf("in place, %d bytes spare", c.oldLen-len(c.source))
		if c.appended {
			how = fmt.Sprintf("appended, grew %d bytes", len(c.source)-c.oldLen)
		}
		report = append(report, fmt.Sprintf("%s: %d replacements, %d -> %d bytes, %s; bytecode, moduleInfo and source hash dropped", c.module, c.replaces, c.oldLen, len(c.source), how))
	}
	report = append(report, fmt.Sprintf(".bun %d -> %d bytes, file %+d bytes, flags %#x -> %#x", b.sec.Size, 8+len(payload), delta, g.flags, le.Uint32(payload[len(payload)-len(trailer)-4:])))
	return out, report, nil
}

func run(path string, sets []string) error {
	patches, err := loadPatches(sets)
	if err != nil {
		return err
	}
	file, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, report, err := patchFile(file, patches)
	if err != nil {
		return fmt.Errorf("%s: %v", path, err)
	}
	for _, line := range report {
		fmt.Println("bunpatch: " + line)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".bunpatch")
	if err := os.WriteFile(tmp, out, info.Mode()); err != nil {
		return err
	}
	if err := os.Chmod(tmp, info.Mode()); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: bunpatch <bun executable> <patches.json>...")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "bunpatch: "+err.Error())
		os.Exit(1)
	}
}
