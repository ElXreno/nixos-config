#!/usr/bin/env python3
"""Branch ledger for an exhaustive sweep.

A branch closes only against a measurement. `close` refuses without one, which is
the point: it makes "I reasoned it was unlikely" impossible to record as closed.

  init   <file> --topic "why the battery drains on idle"
  add    <file> <id> "<branch>" [--why "<why it could contribute>"]
  probe  <file> <id> --measurement "<cmd>" --result "<what it showed>"
  close  <file> <id> --verdict ruled-out|contributing --measurement "<cmd>" --evidence "<result>"
  reopen <file> <id> --why "<what new information reopened it>"
  status <file>
  table  <file>
"""

import argparse
import json
import pathlib
import sys
from typing import Any

OPEN = ("untested", "probing")
VERDICTS = ("ruled-out", "contributing", "inconclusive")
VOLATILE = ("tmpfs", "ramfs", "devtmpfs")
DEFAULT_MIN = 12


def fstype_for(p: pathlib.Path) -> str:
    q = p.absolute()
    while not q.exists() and q != q.parent:
        q = q.parent
    q = q.resolve()
    mounts = pathlib.Path("/proc/mounts")
    if not mounts.exists():
        return ""
    best, best_type = "", ""
    for line in mounts.read_text().splitlines():
        parts = line.split()
        if len(parts) < 3:
            continue
        mp, ty = parts[1].replace("\\040", " "), parts[2]
        if (str(q) == mp or str(q).startswith(mp.rstrip("/") + "/")) and len(mp) >= len(best):
            best, best_type = mp, ty
    return best_type


def load(path: str) -> dict[str, Any]:
    p = pathlib.Path(path).expanduser()
    if not p.exists():
        sys.exit(f"no ledger at {p} — run `init` first")
    return json.loads(p.read_text())


def save(path: str, data: dict[str, Any]) -> None:
    p = pathlib.Path(path).expanduser()
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps(data, indent=1, ensure_ascii=False))


def find(data: dict[str, Any], bid: str) -> dict[str, Any]:
    for b in data["branches"]:
        if b["id"] == bid:
            return b
    sys.exit(f"no branch {bid!r}; known: {', '.join(b['id'] for b in data['branches']) or 'none'}")


def status_line(data: dict[str, Any]) -> str:
    b = data["branches"]
    floor = data.get("min_branches", DEFAULT_MIN)
    dry = data.get("dry_breadth_checks", 0)
    untested = sum(1 for x in b if x["status"] == "untested")
    probing = sum(1 for x in b if x["status"] == "probing")
    closed = sum(1 for x in b if x["status"] == "closed")
    contributing = sum(1 for x in b if x.get("verdict") == "contributing")
    inconclusive = sum(1 for x in b if x.get("verdict") == "inconclusive")
    done = untested == 0 and probing == 0 and len(b) >= floor and dry >= 1
    blockers: list[str] = []
    if len(b) < floor:
        blockers.append(f"needs {floor - len(b)} more branches")
    if untested or probing:
        blockers.append(f"{untested + probing} still open")
    if dry < 1:
        blockers.append("no clean breadth re-check yet")
    return (
        f"LEDGER {len(b)}/{floor} branches | {closed} closed | {untested} untested | {probing} probing"
        f" | {contributing} contributing | {inconclusive} inconclusive | breadth-dry {dry}"
        f" | EXHAUSTED={'yes' if done else 'no'}"
        + ("" if done else f" ({'; '.join(blockers)})")
    )


def goal_line(data: dict[str, Any], path: str) -> str:
    return (
        f"/goal {data['topic']} — every branch in the ledger at {path} is closed against a "
        f"measurement, i.e. the newest LEDGER line printed in this conversation reads "
        f"EXHAUSTED=yes, and the final turn states either which branches are contributing "
        f"with their numbers, or that all branches closed ruled-out. EXHAUSTED only turns yes "
        f"once at least {data.get('min_branches', DEFAULT_MIN)} branches exist, none are open, "
        f"and a breadth re-check has come back clean. Print the LEDGER status line every turn. "
        f"Do not close a branch by reasoning. Stop after 60 turns."
    )


def cmd_init(a: argparse.Namespace) -> None:
    p = pathlib.Path(a.file).expanduser()
    if p.exists():
        sys.exit(f"{p} already exists — use `add`, or pick another path")
    fs = fstype_for(p)
    if fs in VOLATILE:
        sys.exit(
            f"refusing to put the ledger on {fs} ({p}). A sweep outlives reboots and the "
            f"reboot may be what you are investigating. Use a persistent path such as "
            f"~/.claude/investigations/."
        )
    data: dict[str, Any] = {
        "topic": a.topic,
        "branches": [],
        "min_branches": a.min,
        "dry_breadth_checks": 0,
    }
    save(a.file, data)
    print(f"ledger created at {p}")
    print(status_line(data))
    print("\n--- give the user this line to paste, verbatim ---")
    print(goal_line(data, str(p)))
    print("--- end ---")


def cmd_goal(a: argparse.Namespace) -> None:
    data = load(a.file)
    print(goal_line(data, str(pathlib.Path(a.file).expanduser())))


def cmd_add(a: argparse.Namespace) -> None:
    data = load(a.file)
    if any(b["id"] == a.id for b in data["branches"]):
        sys.exit(f"branch {a.id!r} already exists")
    data["branches"].append(
        {
            "id": a.id,
            "branch": a.branch,
            "why": a.why or "",
            "status": "untested",
            "verdict": None,
            "probes": [],
        }
    )
    data["dry_breadth_checks"] = 0
    save(a.file, data)
    print(status_line(data))


def require_floor(data: dict[str, Any]) -> None:
    floor = data.get("min_branches", DEFAULT_MIN)
    n = len(data["branches"])
    if n < floor:
        sys.exit(
            f"only {n} branches enumerated, floor is {floor}. Finish the tree before measuring "
            f"anything — the first branch you probe is the one you will anchor on. Add the "
            f"missing branches, or lower the floor deliberately with `floor --set N` and say why."
        )


def cmd_floor(a: argparse.Namespace) -> None:
    data = load(a.file)
    data["min_branches"] = a.set
    save(a.file, data)
    print(status_line(data))


def cmd_breadth(a: argparse.Namespace) -> None:
    data = load(a.file)
    if a.added > 0:
        data["dry_breadth_checks"] = 0
        print(f"{a.added} new branches found — add them, then re-check.")
    else:
        data["dry_breadth_checks"] = data.get("dry_breadth_checks", 0) + 1
    save(a.file, data)
    print(status_line(data))


def cmd_probe(a: argparse.Namespace) -> None:
    data = load(a.file)
    require_floor(data)
    b = find(data, a.id)
    b["probes"].append({"measurement": a.measurement, "result": a.result})
    if b["status"] == "untested":
        b["status"] = "probing"
    save(a.file, data)
    print(status_line(data))


def cmd_close(a: argparse.Namespace) -> None:
    data = load(a.file)
    require_floor(data)
    b = find(data, a.id)
    if not a.measurement.strip() or not a.evidence.strip():
        sys.exit("close needs both --measurement and --evidence; a branch never closes on reasoning")
    b["probes"].append({"measurement": a.measurement, "result": a.evidence})
    b["status"] = "closed"
    b["verdict"] = a.verdict
    save(a.file, data)
    print(status_line(data))


def cmd_reopen(a: argparse.Namespace) -> None:
    data = load(a.file)
    b = find(data, a.id)
    b["status"] = "probing"
    b["verdict"] = None
    b["why"] = (b["why"] + " | reopened: " + a.why).strip(" |")
    data["rounds_without_new_branch"] = 0
    save(a.file, data)
    print(status_line(data))


def cmd_status(a: argparse.Namespace) -> None:
    data = load(a.file)
    print(status_line(data))
    stuck = [b for b in data["branches"] if b["status"] in OPEN]
    if stuck:
        print("\nstill open:")
        for b in stuck:
            print(f"  [{b['status']:8s}] {b['id']:24s} {b['branch']}")


def cmd_table(a: argparse.Namespace) -> None:
    data = load(a.file)
    print(f"# {data['topic']}\n")
    print("| branch | status | verdict | measurement | result |")
    print("| --- | --- | --- | --- | --- |")
    for b in data["branches"]:
        last = b["probes"][-1] if b["probes"] else {"measurement": "", "result": ""}
        print(
            f"| {b['branch']} | {b['status']} | {b['verdict'] or ''} "
            f"| `{last['measurement']}` | {last['result']} |"
        )
    print(f"\n{status_line(data)}")


def main() -> None:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)

    i = sub.add_parser("init"); i.add_argument("file"); i.add_argument("--topic", required=True); i.add_argument("--min", type=int, default=DEFAULT_MIN); i.set_defaults(fn=cmd_init)
    fl = sub.add_parser("floor"); fl.add_argument("file"); fl.add_argument("--set", type=int, required=True); fl.set_defaults(fn=cmd_floor)
    br = sub.add_parser("breadth"); br.add_argument("file"); br.add_argument("--added", type=int, required=True); br.set_defaults(fn=cmd_breadth)
    a = sub.add_parser("add"); a.add_argument("file"); a.add_argument("id"); a.add_argument("branch"); a.add_argument("--why"); a.set_defaults(fn=cmd_add)
    pr = sub.add_parser("probe"); pr.add_argument("file"); pr.add_argument("id"); pr.add_argument("--measurement", required=True); pr.add_argument("--result", required=True); pr.set_defaults(fn=cmd_probe)
    c = sub.add_parser("close"); c.add_argument("file"); c.add_argument("id"); c.add_argument("--verdict", required=True, choices=VERDICTS); c.add_argument("--measurement", required=True); c.add_argument("--evidence", required=True); c.set_defaults(fn=cmd_close)
    r = sub.add_parser("reopen"); r.add_argument("file"); r.add_argument("id"); r.add_argument("--why", required=True); r.set_defaults(fn=cmd_reopen)
    s = sub.add_parser("status"); s.add_argument("file"); s.set_defaults(fn=cmd_status)
    g = sub.add_parser("goal"); g.add_argument("file"); g.set_defaults(fn=cmd_goal)
    t = sub.add_parser("table"); t.add_argument("file"); t.set_defaults(fn=cmd_table)

    args = p.parse_args()
    args.fn(args)


if __name__ == "__main__":
    main()
