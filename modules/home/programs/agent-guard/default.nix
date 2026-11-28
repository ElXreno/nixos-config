{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    getExe
    mkEnableOption
    mkIf
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.agent-guard;
  hm = config;
  home = config.home.homeDirectory;
  roots = types.listOf types.str;

  readOnlyRoot = types.submodule {
    options = {
      path = mkOption {
        type = types.str;
        description = "Absolute directory the harness may read but not write.";
      };
      reason = mkOption {
        type = types.str;
        description = "Sentence the refusal ends with.";
      };
    };
  };

  deniedWrite = types.submodule {
    options = {
      path = mkOption {
        type = types.str;
        description = "Absolute path, or directory, no command may write to, on this host or over ssh, in a container or in an sh -c payload.";
      };
      reason = mkOption {
        type = types.str;
        description = "Sentence the refusal ends with.";
      };
    };
  };

  harness =
    { name, config, ... }:
    {
      options = {
        harness = mkOption {
          type = types.enum [
            "claude-code"
            "pi"
          ];
          default = name;
          description = "Codec this instance speaks.";
        };
        journal = mkOption {
          type = types.str;
          default = "${hm.xdg.stateHome}/agent-guard/${name}.jsonl";
          description = "Journal every decision is appended to.";
        };
        transcripts = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Transcript directory read by corpus and edits.";
        };
        shellReadRoots = mkOption {
          type = roots;
          default = [ ];
          description = "Roots added to the shared shellReadRoots.";
        };
        createRoots = mkOption {
          type = roots;
          default = [ ];
          description = "Roots added to the shared createRoots.";
        };
        readOnlyRoots = mkOption {
          type = types.listOf readOnlyRoot;
          default = [ ];
          description = "Directories every edit, write and shell redirect into is refused.";
        };
        memoryPaths = mkOption {
          type = roots;
          default = [ ];
          description = "Memory directories the file checks skip; a relative pattern starts at the git root.";
        };
        agentStores = mkOption {
          type = roots;
          default = [ ];
          description = "Directories holding the memory of other agents; mem --as, MEM_AGENT and any read of them ask the owner first. Left empty, the check is off.";
        };
        evidenceTools = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Globs over tool names whose REA Evidence results drop the limitations and envelope this agent was already shown, a raw_result equal to normalized_result and empty fields, merge limitations that differ in one address, and move a multi-line normalized_result after the JSON as plain text; Claude Code only.";
        };
        configFile = mkOption {
          type = types.path;
          readOnly = true;
          internal = true;
          default =
            pkgs.runCommand "agent-guard-${name}.json"
              {
                json = builtins.toJSON {
                  inherit (config)
                    harness
                    journal
                    transcripts
                    readOnlyRoots
                    memoryPaths
                    evidenceTools
                    agentStores
                    ;
                  inherit home;
                  inherit (cfg)
                    scratchBase
                    reposRoot
                    avoidRoots
                    protectedRepos
                    deniedWrites
                    ;
                  inherit (cfg.package) grammars;
                  power = { inherit (cfg.power) enable heavyCommands; };
                  shellReadRoots = cfg.shellReadRoots ++ config.shellReadRoots;
                  createRoots = cfg.createRoots ++ config.createRoots;
                };
                passAsFile = [ "json" ];
              }
              ''
                ${getExe cfg.package} --config "$jsonPath" config check
                cp "$jsonPath" $out
              '';
          description = "Validated configuration of this instance.";
        };
        command = mkOption {
          type = types.str;
          readOnly = true;
          internal = true;
          default = "${getExe cfg.package} --config ${config.configFile}";
          description = "Guard invocation for this instance, without the subcommand.";
        };
      };
    };

  wrapper = pkgs.symlinkJoin {
    name = "agent-guard";
    paths = [ cfg.package ];
    nativeBuildInputs = [ pkgs.makeWrapper ];
    postBuild = ''
      wrapProgram $out/bin/agent-guard \
        --set-default AGENT_GUARD_CONFIG ${cfg.harnesses.${cfg.defaultHarness}.configFile}
    '';
  };
in
{
  options.${namespace}.programs.agent-guard = {
    package = mkOption {
      type = types.package;
      default = pkgs.${namespace}.agent-guard;
      description = "agent-guard package.";
    };
    scratchBase = mkOption {
      type = types.str;
      default = "/mnt/scratch/claude-code";
      description = "Base of the per-session scratch directories.";
    };
    reposRoot = mkOption {
      type = types.str;
      default = "${home}/projects/repos";
      description = "Where repository clones live.";
    };
    avoidRoots = mkOption {
      type = roots;
      default = [
        home
        "/tmp"
        "/mnt/datalake"
      ];
      description = "Places the location advice names as wrong for new files.";
    };
    deniedWrites = mkOption {
      type = types.listOf deniedWrite;
      default =
        map
          (path: {
            inherit path;
            reason = "dropping caches or compacting memory is never done without asking, the patched ZFS ARC gives memory back on demand; work out how much memory is needed, tell the owner and wait.";
          })
          [
            "/proc/sys/vm/drop_caches"
            "/proc/sys/vm/compact_memory"
          ];
      description = "Paths a redirect, tee, cp or sysctl -w is refused for, also inside ssh, container and sh -c payloads, where readOnlyRoots does not look; a sysctl key maps to its /proc/sys path.";
    };
    askTimeoutMinutes = mkOption {
      type = types.ints.between 0 35791;
      default = 10;
      description = "Minutes Claude Code keeps any permission prompt open before denying it, 0 for no limit; questions to the user and plan mode prompts wait, prompts with their own deadline keep it, and a timed prompt is answered only in the terminal, never through Remote Control or channels; exported as AGENT_GUARD_ASK_TIMEOUT_MS.";
    };
    protectedRepos = mkOption {
      type = types.listOf types.str;
      default = [ "github.com/ElXreno/nixos-config" ];
      description = "Repositories a git push into asks for: globs over the remote URL reduced to host/path, with scheme, user, port and .git dropped and case ignored.";
    };
    power = {
      enable = mkEnableOption "battery notes on heavy builds and power source changes" // {
        default = true;
      };
      heavyCommands = mkOption {
        type = types.listOf types.str;
        default = [
          "cargo build"
          "cargo test"
          "cmake --build"
          "docker build"
          "docker buildx build"
          "go build -race"
          "go test -race"
          "make --jobs*"
          "make -j*"
          "nh home build"
          "nh home switch"
          "nh os boot"
          "nh os build*"
          "nh os switch"
          "nh os test"
          "ninja !-t*"
          "nix build"
          "nix flake check"
          "nix-build"
          "nixos-rebuild boot"
          "nixos-rebuild build*"
          "nixos-rebuild dry-activate"
          "nixos-rebuild switch"
          "nixos-rebuild test"
          "podman build"
          "rom build"
        ];
        description = "Commands the battery note calls heavy, each word a glob: the program, then its leading subcommand words in order (an option's value may sit between them), options found anywhere before --, and !word for arguments that rule the command out; --help, -h, --version, --dry-run and --no-build always do. Wrappers, nix develop -c and sh -c payloads are seen through, ssh and container payloads skipped.";
      };
    };
    shellReadRoots = mkOption {
      type = roots;
      default = [
        "/proc"
        "/sys"
        "/dev"
        "/etc"
        "/usr"
        "/nix/store"
        "/run/current-system"
      ];
      description = "Roots the shell may read with cat and friends.";
    };
    createRoots = mkOption {
      type = roots;
      default = [ ];
      description = "Roots where new files are fine besides scratchBase, reposRoot and the git root.";
    };
    harnesses = mkOption {
      type = types.attrsOf (types.submodule harness);
      default = { };
      description = "One guard instance per coding agent harness.";
    };
    defaultHarness = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = "Instance the agent-guard command on PATH uses.";
    };
  };

  config = mkIf (cfg.defaultHarness != null) {
    home.packages = [ wrapper ];
  };
}
