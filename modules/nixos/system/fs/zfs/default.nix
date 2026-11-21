{
  config,
  inputs,
  namespace,
  lib,
  pkgs,
  utils,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    optionals
    mapAttrs
    mapAttrs'
    mapAttrsToList
    nameValuePair
    replaceStrings
    unique
    concatMap
    optional
    any
    attrValues
    length
    head
    ;
  cfg = config.${namespace}.system.fs.zfs;

  faultProbe = pkgs.writeScript "zfs-mmap-fault-probe" ''
    #!${pkgs.python3Minimal}/bin/python3
    import mmap, os, resource, sys
    fd = os.open(sys.argv[1], os.O_RDONLY)
    os.posix_fadvise(fd, 0, 1 << 20, os.POSIX_FADV_DONTNEED)
    m = mmap.mmap(fd, 1 << 20, prot=mmap.PROT_READ)
    before = resource.getrusage(resource.RUSAGE_SELF).ru_majflt
    sum(m[off] for off in range(0, 1 << 16, 4096))
    print(resource.getrusage(resource.RUSAGE_SELF).ru_majflt - before)
    m.close()
    os.posix_fadvise(fd, 0, 1 << 20, os.POSIX_FADV_DONTNEED)
  '';

  zfsDiskDirs = unique (
    concatMap (
      disk:
      optional (any (p: (p.content.type or null) == "zfs") (
        attrValues (disk.content.partitions or { })
      )) (dirOf disk.device)
    ) (attrValues config.disko.devices.disk)
  );

  mobilityPatches = inputs.zfs-slab-mobility.patches.zfs;

  extraPatches = optionals cfg.arcFolioLru mobilityPatches.arclru;

  withPatches =
    zfs:
    if extraPatches == [ ] then
      zfs
    else
      zfs.overrideAttrs (old: {
        patches = (old.patches or [ ]) ++ extraPatches;
      });
in
{
  options.${namespace}.system.fs.zfs = {
    enable = mkEnableOption "managing zfs" // {
      default = config.disko.devices.zpool != { };
      defaultText = "Whether disko declares a zpool.";
    };
    arcFolioLru = mkEnableOption "keeping clean ARC buffers in the page cache" // {
      default = true;
    };
    hostId = mkOption {
      type = types.strMatching "[0-9a-f]{8}";
      description = "networking.hostId, which ZFS uses to recognise pools imported by this machine.";
    };
    autoSnapshot = {
      enable = mkEnableOption "automatic snapshots" // {
        default = cfg.autoSnapshot.filesystems != { };
        defaultText = "Whether any filesystem is listed in autoSnapshot.filesystems.";
      };
      filesystems = mkOption {
        type = types.attrsOf types.bool;
        default = { };
      };
      retention = mkOption {
        type = types.attrsOf types.str;
        default = { };
        example = {
          "rpool/root-persist/postgres" = "4x15m(keep=all) | 4x1h | 1x1d";
        };
        description = ''
          Pruning grid of a dataset that must not follow the common one, such
          as a database that rewrites itself faster than the common grid lets
          its old blocks go.
        '';
      };
    };
    mmapReadaheadKb = mkOption {
      type = types.attrsOf types.ints.positive;
      default = { };
      example = {
        "/mnt/models" = 1024;
      };
      description = ''
        Readahead for mmap page faults, in KiB, per ZFS mountpoint. ZFS
        registers every mount with none, so a sequential mmap read faults one
        page at a time: 1024 lifts mmap loading of model weights from 1.8 to
        3.1 GB/s. Keep it off datasets holding executables, where 128 already
        slows a cold compiler start by 12%. The backing device is found by
        probing, since ZFS names it by mount sequence only.
      '';
    };
  };

  config = mkIf cfg.enable {
    boot = {
      zfs = {
        devNodes = mkIf (length zfsDiskDirs == 1) (head zfsDiskDirs);
        package = withPatches pkgs.zfs_2_4;
        modulePackage = withPatches config.boot.kernelPackages.zfs_2_4;
      };
      initrd.supportedFilesystems = [ "zfs" ];
      supportedFilesystems = [ "zfs" ];

      kernelParams = [
        "init_on_alloc=0"
      ];
    };

    networking.hostId = cfg.hostId;

    systemd.services = mapAttrs' (
      mountPoint: kb:
      let
        mountUnit = "${utils.escapeSystemdPath mountPoint}.mount";
      in
      nameValuePair "zfs-readahead-${utils.escapeSystemdPath mountPoint}" {
        description = "Set mmap readahead for ${mountPoint}";
        after = [ mountUnit ];
        bindsTo = [ mountUnit ];
        wantedBy = [ mountUnit ];
        path = [ pkgs.findutils ];
        serviceConfig = {
          Type = "oneshot";
          RemainAfterExit = true;
        };
        script = ''
          file=$(find ${mountPoint} -xdev -type f -size +1M -print -quit)
          [ -n "$file" ] || { echo "no file to probe under ${mountPoint}"; exit 0; }
          [ "$(${faultProbe} "$file")" = 16 ] || { echo "$file is cached elsewhere, cannot probe"; exit 0; }
          for bdi in /sys/class/bdi/zfs-*; do
            old=$(cat "$bdi/read_ahead_kb")
            echo 128 > "$bdi/read_ahead_kb"
            if [ "$(${faultProbe} "$file")" -lt 16 ]; then
              echo ${toString kb} > "$bdi/read_ahead_kb"
              echo "${mountPoint} is $bdi, read_ahead_kb ${toString kb}"
              exit 0
            fi
            echo "$old" > "$bdi/read_ahead_kb"
          done
          echo "no ZFS bdi answers for ${mountPoint}"
        '';
      }
    ) cfg.mmapReadaheadKb;

    ${namespace} = mkIf cfg.autoSnapshot.enable {
      ports.zrepl-metrics = 9811;
      services.monitoring.agent.targets.zrepl.port = config.${namespace}.ports.zrepl-metrics;
    };

    services.zrepl = mkIf cfg.autoSnapshot.enable {
      enable = true;
      settings = {
        global.monitoring = [
          {
            type = "prometheus";
            listen = "127.0.0.1:${toString config.${namespace}.ports.zrepl-metrics}";
          }
        ];
        jobs =
          let
            snapJob = name: filesystems: grid: {
              inherit name filesystems;
              type = "snap";
              snapshotting = {
                type = "periodic";
                interval = "15m";
                prefix = "zrepl_";
              };
              pruning.keep = [
                {
                  type = "grid";
                  inherit grid;
                  regex = "^zrepl_";
                }
                {
                  type = "regex";
                  negate = true;
                  regex = "^zrepl_";
                }
              ];
            };
          in
          [
            (snapJob "snapshots" (
              cfg.autoSnapshot.filesystems // mapAttrs (_: _: false) cfg.autoSnapshot.retention
            ) "16x15m(keep=all) | 72x1h | 14x1d | 2x30d")
          ]
          ++ mapAttrsToList (
            dataset: snapJob "snapshots-${replaceStrings [ "/" ] [ "-" ] dataset}" { ${dataset} = true; }
          ) cfg.autoSnapshot.retention;
      };
    };
  };
}
