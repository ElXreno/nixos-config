# Project Guide

## Architecture

- Flake-parts + clan-core based NixOS multi-machine config
- `modules/default.nix` auto-collects all `.nix` files under `modules/nixos/` and `modules/home/`
- `nixpkgs.nix` auto-collects all overlays from `overlays/` and packages from `packages/`
- Home-manager modules receive `inputs`, `namespace`, and `virtual` via `extraSpecialArgs`
- Custom namespace used for options: `config.${namespace}.*`

## Style

- **mkEnableOption descriptions**: keep them short — just "Whether to do X.", no elaboration in the description string
- **Use inherited lib functions**: when `inherit (lib) mkIf ...` exists in `let`, use `mkIf` not `lib.mkIf`

## Key Patterns

- **Overlays for external flake packages**: `overlays/<name>/default.nix` with `{ inputs, ... }: _final: prev: { inherit (inputs.<name>.packages.${prev.stdenv.hostPlatform.system}) <pkg>; }`
- **Home-manager program modules**: `modules/home/programs/<name>/default.nix` — wrap upstream HM options behind `${namespace}.programs.<name>.enable`
- **Roles** (`modules/home/roles/`) aggregate program enables; per-host configs live in `homes/<user>@<host>/default.nix`
- **NixOS service modules**: `modules/nixos/services/<name>/default.nix`

## MCP Servers

- **nixos**: Use `mcp__nixos__nix` for searching NixOS/Home Manager packages and options instead of `nix search`

## Useful Commands

- Check HM module defaults: `nix flake metadata github:nix-community/home-manager --json | jq -r .path` then read the module source from the store path
- Query HM options: use `mcp__nixos__nix` with `source=home-manager`, `type=options`
- System platform in modules: `pkgs.stdenv.hostPlatform.system`
- Read clan-core source: `nix flake metadata github:ElXreno/clan-core --json | jq -r .path` then read from the store path
- No `python3` in the devShell — use `nix shell nixpkgs#python3 -c python3` for one-off scripts

## Nixpkgs Fork Workflow

- Fork lives at `~/projects/repos/github.com/ElXreno/nixpkgs`, branch `nixos-unstable-cust`
- To cherry-pick a nixpkgs PR: `git fetch upstream pull/<PR_NUMBER>/head:<branch_name> && git cherry-pick <branch_name>`
- Then push and run `nix flake update nixpkgs` in this repo to pick up the change

## Networking

- **Tailscale** is the inter-machine network (replaced Yggdrasil). Clan domain is `angora-ide.ts.net`
- **Clan host discovery** uses the `internet` clan service with Tailscale MagicDNS hostnames (lowercase: `bimba`, `grate`, etc.)
- **ncps cache** resolves as `http://BIMBA.angora-ide.ts.net:8501`
- **Tailscale overlay** patches `socketBufferSize` from 7MB to 16MB to eliminate UDP recv buffer drops
- **bpftune** runs on all machines with `sysctl_tuner` excluded (it conflicts with NixOS sysctl re-application on rebuild)
- **TCP buffer tuning**: desktop `tcp_rmem`/`tcp_wmem` max = 64MB, `rmem_max`/`wmem_max` = 128MB — needed because wireguard-go packet reordering causes TCP rcv_ssthresh collapse with smaller buffers

## Common Pitfalls

- **New files must be `git add`ed immediately after creation**: Nix flakes only see git-tracked files — always run `git add` right after creating any new `.nix` file (overlays, modules, disko configs, etc.). Do not wait for the user to discover this
- **Impermanence**: every new service needs its state dirs manually added to impermanence config — missing one causes silent runtime failures after deploy
- **Clan vars are runtime-only**: if a NixOS module needs a secret value at Nix eval time (e.g. hashed passwords in `serverConfig`), generate the derived value in the clan generator and expose it as a non-secret `.value`
- **`virtual` arg**: use `virtual` (passed via `extraSpecialArgs`) to gate VM-only configuration in modules
- **NixOS option priority**: clan-core sets defaults with `mkDefault` — override with plain assignment (priority 100), not `mkForce`, unless semantics require it
- **Service restarts**: `nixos-rebuild switch` doesn't always restart services whose configs changed — verify with `systemctl show -p ActiveEnterTimestamp <unit>`
