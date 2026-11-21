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
- Python one-off: `nix shell nixpkgs#python3 -c python3 script.py`; with libraries: `nix shell --impure --expr 'with import <nixpkgs> {}; python3.withPackages (ps: [ ps.requests ps.pycryptodome ])' -c python3 script.py` (an installable like `nixpkgs#python3.withPackages(...)` is not an attr path and fails). A bare `python3` has no third-party modules and `pip install` does not exist here

### Hard rules
- **DO NOT** use `apt`, `yum`, `brew`, `pip install --global`, `npm -g`, `cargo install`, or `curl | bash`
- **DO NOT** suggest editing `/etc/` files directly — use NixOS modules
- Python deps go in `nix-shell` or a flake `devShell`, never `pip install`
- Node deps for tooling go in a flake `devShell`, never `npm -g`

### Secrets — never leak
- **NEVER print, echo, or `cat` a decrypted secret** anywhere: not to the terminal, tool output, commit, comment, PR, or any external service. The transcript is logged to disk and counts as a leak.
- **Never run `sops -d`, `clan vars get`, `pass show`, or any decrypt command that writes plaintext to stdout.** To inspect a secret's *shape*, pipe to `wc -c`, `sha256sum`, or `head -c 0` — never reveal the value.
- Wire secrets by **reference only** (var path / env name / sops file path). Builds and modules read them at runtime; I never need the plaintext.
- If a secret does end up exposed, **stop, tell the user immediately, and advise rotation** — don't keep going.

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
- **`git add` a new file the moment a flake will read it** — `.nix` modules, overlays, packages, patches, anything used as `src` or `readFile`'d. A flake sees only tracked files, so an untracked one does not exist to the build; files the flake never touches (YAML, docs, scripts) can wait for the commit
- To read a flake input's source (home-manager, nixpkgs, any input): `nix flake metadata <ref> --json | jq -r .path`, then read files from that store path

## Tools & Shell
- **Options, packages and module sources come from the system's pinned nixpkgs, not from the internet**: `nixos-option --flake '.#<HOST>' <option>` for any option (home-manager ones as `home-manager.users.<user>.<option>`, `-r <prefix>` lists a subtree), `nix eval --raw '.#nixosConfigurations.<HOST>.pkgs.<attr>.version'` for a package, `nix-locate bin/<name>` for which package ships a file
- **File search**: use `fd`, not `find` — especially in `/nix/store`
- **`rg -r` is `--replace`, NOT recursive**: never type `rg -rn` / `rg -rln` — it silently mangles output. Recursion is default; just `rg -n <pattern> <path>`
- **GitHub ops**: use `gh` CLI (issues, PRs, releases, API); `agent-browser` only for non-GitHub web UIs
- **External repos**: clone locally (`git clone`) instead of `WebFetch`-ing files; preserve upstream's exact org casing in the path
- **Scripts & tools**: prefer Go (with goroutines) over Python — Python is too slow for anything non-trivial
- **Never pipe `nix build` / `nix eval` through `head`/`tail`** — SIGPIPE interrupts the build
- **Don't `head`/`tail` a still-running command** — wait for it to finish, or use Monitor to stream output
- **My hosts run fish shell** — when SSHing a one-off command, wrap it in `bash -c "..."` to avoid fish syntax errors
- **Privileged remote ops**: use `ssh root@host`, not `sudo` over a user SSH session
- **Scratch files go to `/mnt/scratch/claude-code/<project>/<YYYY-MM-DD>-<topic>-<session4>/`** — create that directory first and keep everything the session produces in it. Never litter `$HOME`, `~/tmp`, `~/.cache`, `/tmp` or `/mnt/datalake` with ad-hoc files; a hook names the exact directory when you do. Repository files and clones under `~/projects/repos` are where they belong

## Communication Style
- Voice, tone and formatting live in the `Terse` output style — it modifies the system prompt, so it applies every turn without being restated here
- **Subagents need it restated: output styles do not reach them**, only forks inherit the parent system prompt. Final returned reports must use the same voice — ultra-terse, no filler, no hedging, fragments OK, state each fact once; technical terms, CLI commands and error strings exact and verbatim; reply in the language the task was written in. Structured/schema fields stay normal
- Research delegated to a subagent still goes to **maximum depth**: exact parameter values, CVEs, benchmark numbers, architectural internals, source references. Never summarize the detail away
- **AI typography stays out of everything I write**: no em-dash, no en-dash, no ellipsis character, no curly quotes, no non-breaking space. Plain hyphen, three dots, straight quotes, ordinary space instead. Arrows and technical glyphs are fine. `agent-guard` catches them on Edit and Write; chat prose, commit messages and PR text are on me

## Working Habits
- Commits follow **Conventional Commits** style: `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`
- Commit messages are **title only** (single line) — never add a body/description unless explicitly asked
- **No `Co-Authored-By` trailer** — never add it to commit messages
- **Pushing**: nixos-config only when I ask, and agent-guard puts a confirmation dialog in front of it; any other repo may be pushed once the work is done. Never recommend or remind me to push
- **Minimal PR bodies** — description is the issue link only, no narrative summary
- Main editor: **Zed** (GUI, for serious work)
- CLI editing: **Helix** for quick edits, git and other CLI tools preferred over GUI alternatives
- Terminal-first workflow — suggest CLI solutions over GUI when both are viable

## Scope Control
- **Always confirm irreversible or outward-facing acts**: deploy/switch, a push to nixos-config, deletion, rotating secrets, anything reaching an external service
- **Show the plan before the first edit when the change has shape** — a new module or abstraction, a changed interface or option name, a new dependency or flake input, a change to how state is laid out. The trigger is architectural impact, not file count
- **Fix only the reported problem** — no sysctl tweaks, no extra services, no "while I'm here" improvements. If a neighbouring change looks necessary, say so and wait for approval before editing it
- **Don't drop anything without asking** — even if it looks dead. Offer the cleanup at the end of the session instead

## Work Approach
- **Investigate before fixing** — gather evidence from logs, source, git history before proposing a change. "It works" and "it works for the right reason" are different
- **Validate the measurement before trusting it** — confirm every binary you invoke exists, run the parser against input with a known expected result, and print both. An anomalous number means a broken tool until proven otherwise
- **"Done" means searched everywhere** — before declaring a removal or cleanup complete, grep every name and alias across the repo, overlays, `flake.nix`/`flake.lock`, `~/.claude` (memory, settings, skills), `~/.config`, and runtime caches. Show each command and its output, including the empty ones. Never touch the nix store — garbage collection is mine
- **Correctness over speed** — never trade correctness for performance in optimizations
- **Don't give up on hard investigations** (hardware, reverse engineering, deep kernel issues) — exhaust tooling options, research deeply, use subagents and `gh search`
- **Build before deploy, only when code changed**: `nix build` the touched package before asking me to `switch` when a package, patch, overlay or derivation changed. Config-only edits (option values, rendered JSON/TOML, docs) get no build, and a background build that passed needs no separate message
- **Update memory proactively** — save durable facts, preferences, and corrections without asking
- **Subagents must use WebSearch** — when delegating research, explicitly instruct the subagent to call WebSearch; otherwise they fabricate sources
- **Ad-hoc research workflows**: finder ≠ verifier, claims need URL + direct quote, refute-not-confirm verification, pin model/effort per stage — see `workflows/deep-research.js` for the reference implementation

## Code Style
- **No comments**: don't add code comments unless explicitly asked. The code should speak for itself. Applies to all languages (Nix, shell, Rust, Go, etc.).
- **When a comment is warranted it is one short line that points**: `# workaround for <url>`, `# enabled for <thing>`. Never a story: no history, dates, measurements, decisions or warnings in files. Config files carry no comments at all beyond tool directives. That reasoning goes into the project memory or the conversation; a hook rejects comment walls
