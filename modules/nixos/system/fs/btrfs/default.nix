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
  cfg = config.${namespace}.system.fs.btrfs;

  pathOnMount =
    path: mount:
    let
      m = if mount == "/" then "/" else mount + "/";
      p = path + "/";
    in
    lib.hasPrefix m p;

  mountFor =
    path:
    lib.foldl' (a: b: if lib.stringLength b > lib.stringLength a then b else a) "/" (
      lib.filter (m: pathOnMount path m) (lib.attrNames config.fileSystems)
    );

  impCfg = config.${namespace}.system.impermanence;

  isImpermanenceBound =
    path:
    impCfg.enable && lib.any (d: (if lib.isAttrs d then d.directory else d) == path) impCfg.directories;

  effectiveMountFor =
    path: if isImpermanenceBound path then mountFor impCfg.defaultPersistentPath else mountFor path;

  isBtrfsPath = path: (config.fileSystems.${effectiveMountFor path}.fsType or null) == "btrfs";

  # Target the persist-root source for impermanence binds; chattr operates on inodes.
  canonicalPathFor =
    path: if isImpermanenceBound path then "${impCfg.defaultPersistentPath}${path}" else path;

  effectiveNocowPaths = lib.filter (
    path:
    lib.warnIf (!isBtrfsPath path) "btrfs.nocowPaths: ${path} is not on btrfs, skipping" (
      isBtrfsPath path
    )
  ) cfg.nocowPaths;

  btrfsReclaimSetupScript = pkgs.writeShellScriptBin "btrfs-reclaim-setup.sh" ''
    set -ex

    FSID="$1"
    THRESH="10"
    PERIODIC="1"
    RETRIES="3"
    SLEEP_SECS="3"

    if [ -z "$FSID" ]; then
      echo "FSID not provided"
      exit 1
    fi

    BASE="/sys/fs/btrfs/$FSID/allocation/data"
    TH_FILE="$BASE/bg_reclaim_threshold"
    PR_FILE="$BASE/periodic_reclaim"

    attempt=1
    while [ "$attempt" -le "$RETRIES" ]; do
      if [ -w "$TH_FILE" ] && [ -w "$PR_FILE" ]; then
        echo "$THRESH" > "$TH_FILE"
        echo "$PERIODIC" > "$PR_FILE"

        cur_th="$(cat "$TH_FILE")"
        cur_pr="$(cat "$PR_FILE")"
        if [ "$cur_th" = "$THRESH" ] && [ "$cur_pr" = "$PERIODIC" ]; then
          exit 0
        fi
      fi

      if [ "$attempt" -lt "$RETRIES" ]; then
        sleep "$SLEEP_SECS"
      fi
      attempt=$((attempt + 1))
    done

    exit 1
  '';

  btrfsReclaimSetupTraverseScript = pkgs.writeShellScriptBin "btrfs-reclaim-setup-traverse.sh" ''
    set -ex
    for fs in /sys/fs/btrfs/*; do
      [ -d "$fs/allocation/data" ] || continue
      FSID=$(basename "$fs")
      ${lib.getExe btrfsReclaimSetupScript} "$FSID"
    done
  '';
in
{
  options.${namespace}.system.fs.btrfs = {
    enable = mkEnableOption "Whether or not to manage btrfs." // {
      default = lib.any (x: x.fsType == "btrfs") config.system.build.fileSystems;
    };
    nocowPaths = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = "Paths to mark NoCoW (`chattr +C`) via tmpfiles. Only fires for btrfs paths. New files inherit; existing files need a manual mv+cp dance.";
    };
  };

  config = mkIf cfg.enable {
    systemd.tmpfiles.rules = map (path: "h ${canonicalPathFor path} - - - - +C") effectiveNocowPaths;

    systemd.services."btrfs-balance-auto" = {
      wantedBy = [ "sysinit.target" ];
      script = ''
        ${lib.getExe btrfsReclaimSetupTraverseScript}
      '';
      serviceConfig = {
        Type = "oneshot";
        User = "root";
      };
    };

    environment.systemPackages = [ pkgs.duperemove ];
  };
}
