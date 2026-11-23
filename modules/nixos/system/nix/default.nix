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
    mkPackageOption
    mkMerge
    optional
    ;
  cfg = config.${namespace}.system.nix;

  cacheProxy = lib.importTOML ../../../../cf-workers/nix-cache-proxy/wrangler.toml;
in
{
  options.${namespace}.system.nix = {
    enable = mkEnableOption "managing nix" // {
      default = true;
    };
    package = mkPackageOption pkgs "nix-optimized" { };
    auto-optimise.enable = mkEnableOption "automatic store optimisation";
    gc.enable = mkEnableOption "automatic garbage collection";

    evalHeapSizeMiB = lib.mkOption {
      type = lib.types.nullOr lib.types.ints.positive;
      default =
        let
          totalMiB = config.${namespace}.facts.memory.totalMiB;
        in
        if totalMiB == 0 then null else lib.min 4096 (totalMiB / 8);
      defaultText = "An eighth of total RAM, capped at 4096. Null when facter reports no memory.";
      description = ''
        Initial Boehm GC heap for Nix evaluation, in MiB, exported as
        `GC_INITIAL_HEAP_SIZE`. Evaluation runs client-side, so this applies to
        user sessions rather than nix-daemon. A heap large enough to hold the
        whole evaluation cuts CPU time by roughly a third; resident memory still
        tracks actual allocation, so the reservation itself is free. Null leaves
        Nix's own default in place.
      '';
    };

    buildDir = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "/mnt/scratch/nix-builds";
      description = ''
        Directory holding derivation build scratch, bind-mounted into the
        sandbox as `$TMPDIR`. Since Nix 2.30 this defaults to
        `/nix/var/nix/builds`, so on a deduplicated or encrypted store dataset
        every ephemeral build file pays that cost and is then discarded.
        Neither this directory nor any of its parents may be world-writable.
      '';
    };
  };

  config = mkIf cfg.enable {
    nix = {
      inherit (cfg) package;

      channel.enable = false;

      daemonCPUSchedPolicy = "other";
      daemonIOSchedClass = "best-effort";

      settings = mkMerge [
        {
          auto-optimise-store = cfg.auto-optimise.enable;

          experimental-features = [
            "auto-allocate-uids"
            "cgroups"
          ];

          system-features = [ "uid-range" ];

          auto-allocate-uids = true;

          substituters =
            optional (!config.${namespace}.services.nix-farm.enable)
              "${config.${namespace}.services.niks3.url}?priority=10"
            ++ [ "https://${(lib.head cacheProxy.routes).pattern}?priority=11" ];
          trusted-public-keys = [
            config.${namespace}.services.niks3.publicKey
          ]
          ++ map (upstream: upstream.key) cacheProxy.vars.UPSTREAMS;

          min-free = 2 * 1024 * 1024 * 1024; # 2GB
          max-free = 5 * 1024 * 1024 * 1024; # 5GB
        }
        (mkIf (cfg.buildDir != null) {
          build-dir = cfg.buildDir;
        })
      ];

      gc = mkIf cfg.gc.enable {
        automatic = true;
        dates = "daily";
        options = "-d --delete-older-than 14d";
      };
    };

    environment.sessionVariables = mkIf (cfg.evalHeapSizeMiB != null) {
      GC_INITIAL_HEAP_SIZE = "${toString cfg.evalHeapSizeMiB}M";
    };

    systemd.tmpfiles.rules = lib.optionals (cfg.buildDir != null) [
      "d ${cfg.buildDir} 0755 root root -"
    ];
  };
}
