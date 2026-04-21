# Global Context

## About Me
I'm a DevOps/Platform Engineer with advanced expertise in Linux system administration
and NixOS/Nix ecosystem. Comfortable with Kubernetes, TalosOS, FluxCD, and OpenTofu
for infrastructure. Know Rust and Go at a basic-to-intermediate level.
Treat me as a senior engineer — skip obvious explanations unless I ask.

## System
I use NixOS. There is no conventional apt/yum/brew/pip here.

### Running software
- One-off tool run: `nix run nixpkgs#<package>`
- Temporary shell with tools: `nix shell nixpkgs#<package> nixpkgs#<package2>`
- Ephemeral environment: `nix-shell -p <packages>`
- Enter project dev environment: `nix develop` (reads `flake.nix`)

### Hard rules
- **DO NOT** use `apt`, `yum`, `brew`, `pip install --global`, `npm -g`, `cargo install`, or `curl | bash`
- **DO NOT** suggest editing `/etc/` files directly — use NixOS modules
- Python deps go in `nix-shell` or a flake `devShell`, never `pip install`
- Node deps for tooling go in a flake `devShell`, never `npm -g`

## Configuration
- System config: declarative via `configuration.nix` or NixOS modules
- User config: Home Manager modules (`home.nix`)
- NixOS config lives at `~/Sync/PC/share/nixos-config`
- All environments pinned via **Nix Flakes** (`flake.nix` + `flake.lock`)
- Update flake inputs: `nix flake update`

## Nix Flakes
- Preferred approach for all dev environments and package pinning
- Dev shell entry: `nix develop`
- Always define `devShells` in `flake.nix` instead of loose install scripts

## Tools & Shell
- **File search**: use `fd`, not `find` — especially in `/nix/store`
- **GitHub ops**: use `gh` CLI (issues, PRs, releases, API); Playwright only for non-GitHub web UIs
- **External repos**: clone locally (`git clone`) instead of `WebFetch`-ing files; preserve upstream's exact org casing in the path
- **Scripts & tools**: prefer Go (with goroutines) over Python — Python is too slow for anything non-trivial
- **Never pipe `nix build` / `nix eval` through `head`/`tail`** — SIGPIPE interrupts the build
- **Don't `head`/`tail` a still-running command** — wait for it to finish, or use Monitor to stream output
- **My hosts run fish shell** — when SSHing a one-off command, wrap it in `bash -c "..."` to avoid fish syntax errors
- **Privileged remote ops**: use `ssh root@host`, not `sudo` over a user SSH session

## Communication Style
- For "compare X vs Y" or research tasks, go **maximum depth**: exact algorithms, parameter values, CVEs, benchmark numbers, architectural internals, threat models, developer positions on open issues
- Don't summarize away details — provide the raw technical substance with decision matrices and source references

## Working Habits
- Commits follow **Conventional Commits** style: `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`
- Commit messages are **title only** (single line) — never add a body/description unless explicitly asked
- **No `Co-Authored-By` trailer** — never add it to commit messages
- **Don't `git push` by default** — after commit, wait for explicit authorization; treat each push as its own action
- **Minimal PR bodies** — description is the issue link only, no narrative summary
- Main editor: **Zed** (GUI, for serious work)
- CLI editing: **Helix** for quick edits, git and other CLI tools preferred over GUI alternatives
- Terminal-first workflow — suggest CLI solutions over GUI when both are viable

## Work Approach
- **Investigate before fixing** — gather evidence from logs, source, git history before proposing a change. "It works" and "it works for the right reason" are different
- **Correctness over speed** — never trade correctness for performance in optimizations
- **Don't give up on hard investigations** (hardware, reverse engineering, deep kernel issues) — exhaust tooling options, research deeply, use subagents and `gh search`
- **Build before deploy** — always `nix build` and verify output before asking me to `switch`
- **Update memory proactively** — save durable facts, preferences, and corrections without asking
- **Subagents must use WebSearch** — when delegating research, explicitly instruct the subagent to call WebSearch; otherwise they fabricate sources

## Code Style
- **No comments**: don't add code comments unless explicitly asked. The code should speak for itself. Applies to all languages (Nix, shell, Rust, Go, etc.).
