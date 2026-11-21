#!/usr/bin/env python3
"""Audit and verify a Claude Code memory directory across an index-format migration.

Subcommands:
  audit <dir>                  report size, byte breakdown, format, orphans, dangling refs
  snapshot <dir> <out.json>    record every reachable file before rewriting the index
  verify <dir> <out.json>      after rewriting: prove nothing was lost and every slug resolves
"""

import argparse
import json
import pathlib
import re
import sys
from dataclasses import dataclass, field

PREFIXES = ("feedback_", "reference_", "project_")
LINK = re.compile(r"\]\(([^)]+\.md)\)")
WIKI = re.compile(r"\[\[([^\]]+)\]\]")
TICK = re.compile(r"`((?:feedback|reference|project)_[a-z0-9_]+)`")
HEADER = re.compile(r"^[^:]*?\((\w+_)\*\):\s*(.*)$")
PLAIN_HEADER = re.compile(r"^[^:]{1,40}:\s*(.*)$")
LEAD = re.compile(r"^([a-z][a-z0-9_]*)")


@dataclass
class Report:
    files: set[str] = field(default_factory=set)
    indexed: set[str] = field(default_factory=set)
    hubs: set[str] = field(default_factory=set)
    body_only: set[str] = field(default_factory=set)
    orphans: set[str] = field(default_factory=set)
    dangling: set[str] = field(default_factory=set)
    unresolved: list[tuple[str, str]] = field(default_factory=list)
    size: int = 0


def load(d: pathlib.Path) -> set[str]:
    return {f.name for f in d.glob("*.md")} - {"MEMORY.md"}


def parse_index_line(line: str, files: set[str]) -> tuple[set[str], list[tuple[str, str]]]:
    hits: set[str] = set()
    bad: list[tuple[str, str]] = []
    if not line.startswith("- "):
        return hits, bad
    body, pfx = line[2:], ""
    m = HEADER.match(body)
    if m:
        pfx, body = m.group(1), m.group(2)
    else:
        m2 = PLAIN_HEADER.match(body)
        if m2:
            body = m2.group(1)
    for name in LINK.findall(body):
        hits.add(name)
    body = LINK.sub(" ", body)
    body = re.sub(r"\[([^\]]*)\]", r" ", body)
    for chunk in re.split(r" · | \+ | → ", body):
        item = re.split(r" — ", chunk)[0].strip().strip("`")
        tok = LEAD.match(item)
        if not tok:
            continue
        s = tok.group(1)
        cand = s if s.startswith(PREFIXES) or s == "archive" else pfx + s
        if cand + ".md" in files:
            hits.add(cand + ".md")
        elif s.startswith(PREFIXES) or pfx:
            bad.append((s, cand + ".md"))
    return hits, bad


def index_refs(path: pathlib.Path, files: set[str]) -> tuple[set[str], list[tuple[str, str]]]:
    hits: set[str] = set()
    bad: list[tuple[str, str]] = []
    for line in path.read_text().splitlines():
        h, b = parse_index_line(line, files)
        hits |= h
        bad += b
    return hits, bad


def hub_refs(path: pathlib.Path) -> set[str]:
    t = path.read_text()
    return {n + ".md" for n in TICK.findall(t)} | set(LINK.findall(t))


def reachable(d: pathlib.Path) -> Report:
    files = load(d)
    idx = d / "MEMORY.md"
    direct, bad = index_refs(idx, files)
    hubs: set[str] = set()
    for name in sorted(direct):
        f = d / name
        if not f.exists():
            continue
        if len(hub_refs(f) & files) >= 5:
            hubs.add(name)
    via_hub: set[str] = set()
    for name in sorted(hubs):
        via_hub |= hub_refs(d / name) & files
    indexed = (direct | via_hub) & files
    body: set[str] = set()
    for f in d.glob("*.md"):
        if f.name == "MEMORY.md" or f.name in hubs:
            continue
        body |= {n + ".md" for n in WIKI.findall(f.read_text())}
    return Report(
        files=files,
        indexed=indexed,
        hubs=hubs,
        body_only=(body & files) - indexed,
        orphans=files - indexed - body,
        dangling=(direct - files) - {"archive.md"},
        unresolved=bad,
        size=idx.stat().st_size,
    )


def cmd_audit(d: pathlib.Path) -> int:
    r = reachable(d)
    text = (d / "MEMORY.md").read_text()
    links = re.findall(r"\[([^\]]*)\]\(([^)]*)\)", text)
    total = len(text.encode())
    linked_share = len(links) / max(1, len(r.indexed))
    print(f"format            {'OLD (markdown links)' if linked_share > 0.5 else 'NEW (bare slugs)'}")
    print(f"MEMORY.md         {r.size} B")
    print(f"files on disk     {len(r.files)}")
    print(f"indexed           {len(r.indexed)}")
    print(f"hubs detected     {sorted(r.hubs) or 'none'}")
    if links:
        fn = sum(len(f.encode()) + 4 for _, f in links)
        lab = sum(len(l.encode()) for l, _ in links)
        slugs = sum(len(f.encode()) - 3 for _, f in links)
        print(f"markdown links    {len(links)}")
        print(f"  filenames+[]()  {fn} B ({fn / total:.0%})")
        print(f"  labels          {lab} B ({lab / total:.0%})")
        print(f"  bare-slug est.  ~{total - fn - lab + slugs} B; prefix hoisting goes below this")
    print(f"body-only         {len(r.body_only)}  {sorted(r.body_only)}")
    print(f"orphans           {len(r.orphans)}  {sorted(r.orphans)}")
    print(f"dangling          {len(r.dangling)}  {sorted(r.dangling)}")
    print(f"unresolved slugs  {r.unresolved or 'none'}")
    return 0


def cmd_snapshot(d: pathlib.Path, out: str) -> int:
    r = reachable(d)
    want = sorted(r.indexed | r.body_only | r.orphans)
    pathlib.Path(out).write_text(json.dumps({"want": want, "size": r.size}, indent=1))
    print(f"snapshot: {len(want)} files must stay reachable, MEMORY.md was {r.size} B -> {out}")
    return 0


def cmd_verify(d: pathlib.Path, snap: str) -> int:
    base = json.loads(pathlib.Path(snap).read_text())
    want: set[str] = set(base["want"])
    r = reachable(d)
    lost = sorted(want - r.indexed)
    ok = not (lost or r.dangling or r.unresolved)
    print(f"unresolved slugs  {r.unresolved or 'none'}")
    print(f"dangling refs     {sorted(r.dangling) or 'none'}")
    print(f"LOST              {lost or 'none'}")
    print(f"body-only left    {sorted(r.body_only) or 'none'}")
    print(f"orphans left      {sorted(r.orphans) or 'none'}")
    print(f"covered {len(r.indexed)} / wanted {len(want)}")
    print(f"MEMORY.md {base['size']} -> {r.size} B ({r.size / base['size'] - 1:+.0%})")
    print("PASS" if ok else "FAIL")
    return 0 if ok else 1


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    a = sub.add_parser("audit")
    a.add_argument("dir")
    s = sub.add_parser("snapshot")
    s.add_argument("dir")
    s.add_argument("out")
    v = sub.add_parser("verify")
    v.add_argument("dir")
    v.add_argument("snapshot")
    args = p.parse_args()
    d = pathlib.Path(args.dir)
    if not (d / "MEMORY.md").exists():
        print(f"no MEMORY.md in {d}", file=sys.stderr)
        return 2
    if args.cmd == "audit":
        return cmd_audit(d)
    if args.cmd == "snapshot":
        return cmd_snapshot(d, args.out)
    return cmd_verify(d, args.snapshot)


if __name__ == "__main__":
    sys.exit(main())
