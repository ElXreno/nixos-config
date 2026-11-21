{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.system.ccache;

  tools = pkgs.ccacheTools.override { inherit (cfg) cacheDir settings; };
in
{
  options.${namespace}.system.ccache = {
    enable = mkEnableOption "providing the object cache to builds run on this machine";

    cacheDir = mkOption {
      type = types.str;
      default = "/mnt/scratch/ccache";
      description = ''
        Directory holding the cache. Keep it off the `/nix` dataset: that one
        runs blake3 dedup against an 8G table quota, which is wasted work on
        objects ccache already addresses by content.

        Not part of {option}`settings`, because ccache deliberately ignores
        `cache_dir` when it comes from a cache-specific config file.
      '';
    };

    thinltoCacheDir = mkOption {
      type = types.str;
      internal = true;
      readOnly = true;
      default = "${cfg.cacheDir}/thinlto";
      description = ''
        Directory for lld's ThinLTO backend cache, passed to kernel builds as
        `KBUILD_THINLTO_CACHE`. Lives inside {option}`cacheDir` so the sandbox
        already sees it.

        A bindfs view of {option}`thinltoCacheBackingDir`: lld creates its
        entries 0600, and builds run under differing auto-allocated uids, so
        without the view no build could read another's entries.
      '';
    };

    thinltoCacheBackingDir = mkOption {
      type = types.str;
      internal = true;
      readOnly = true;
      default = "${cfg.cacheDir}/.thinlto";
      description = "Where the entries behind {option}`thinltoCacheDir` are stored.";
    };

    aliases = mkOption {
      type = types.listOf types.str;
      default = [ ];
      example = [ "/nix/ccache" ];
      description = ''
        Further paths under which {option}`cacheDir` appears inside the build
        sandbox. A flake that wraps its compiler in ccache has to name the
        directory as a build-time constant, or a derivation built here would
        stop matching one built elsewhere. Mapping the constant onto this cache
        lets such a flake use it without a second copy.
      '';
    };

    owner = mkOption {
      type = types.str;
      default = "root";
      description = ''
        Owner of {option}`cacheDir`. Must match the owner of its parent
        directory: systemd-tmpfiles aborts canonicalization on an "unsafe path
        transition" when descending from a user-owned directory into a
        root-owned one, skipping every rule below it and failing the unit.

        Not a security boundary — the directory is world-writable regardless,
        since builds run under auto-allocated uids that share no group.
      '';
    };

    group = mkOption {
      type = types.str;
      default = "root";
      description = "Group of {option}`cacheDir`. See {option}`owner`.";
    };

    settings = mkOption {
      type =
        with types;
        attrsOf (oneOf [
          bool
          int
          str
        ]);
      default = pkgs.ccacheTools.settings;
      description = ''
        Contents of the generated `ccache.conf`, keyed by ccache's own config
        names. Reached from builds via `CCACHE_CONFIGPATH`, which puts the file
        in the derivation closure, and symlinked into {option}`cacheDir` so a
        manual `ccache -s` reports the same values the builds use.

        `compiler_check` is deliberately absent: it is derived per stdenv and
        passed through the environment, which outranks this file.

        The default `sloppiness` is what makes direct mode work under Nix,
        where every build unpacks its sources afresh and include timestamps
        never match a previous run.
      '';
    };

    wrapStdenv = mkOption {
      type = types.raw;
      internal = true;
      readOnly = true;
      default = tools.wrapStdenv;
      description = ''
        Takes a stdenv and returns it with its unwrapped compiler replaced by a
        ccache shim. Applies to `stdenv.cc.cc` rather than the wrapper because
        kbuild passes `CC=` as an absolute path to the unwrapped compiler.

        Usable from any host's configuration: what matters is the machine that
        runs the build. Where {option}`cacheDir` is not writable inside the
        sandbox the shim sets `CCACHE_DISABLE` and compiles uncached, since
        ccache itself would fail the compile with "Permission denied".
      '';
    };
  };

  config = mkIf cfg.enable {
    systemd.tmpfiles.rules = [
      "d ${cfg.cacheDir} 0777 ${cfg.owner} ${cfg.group} -"
      "d ${cfg.thinltoCacheBackingDir} 0755 ${cfg.owner} ${cfg.group} -"
      "L+ ${cfg.cacheDir}/ccache.conf - - - - ${tools.confFile}"
    ];

    nix.settings.extra-sandbox-paths = [ cfg.cacheDir ] ++ map (p: "${p}=${cfg.cacheDir}") cfg.aliases;

    fileSystems.${cfg.thinltoCacheDir} = {
      device = cfg.thinltoCacheBackingDir;
      fsType = "fuse.bindfs";
      options = [
        "perms=a+rwX"
        "create-for-user=${cfg.owner}"
        "create-for-group=${cfg.group}"
        "create-with-perms=a+rw"
        "multithreaded"
        "nofail"
        "x-systemd.after=systemd-tmpfiles-setup.service"
      ];
    };

    environment.systemPackages = [ pkgs.ccache ];
  };
}
