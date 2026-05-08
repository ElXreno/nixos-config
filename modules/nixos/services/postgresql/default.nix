{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption mkPackageOption;
  cfg = config.${namespace}.services.postgresql;
  inherit (config.${namespace}) facts;

  inherit (facts.memory) totalMiB;
  inherit (facts.cpu) threads;

  # Conservative ratios for shared-tenant hosts (e.g. BIMBA also runs Minecraft).
  # Override per-host via services.postgresql.settings if a dedicated DB host
  # wants more aggressive tuning.
  sharedBuffersMiB = lib.max 128 (totalMiB / 16);
  effectiveCacheSizeMiB = totalMiB * 3 / 16;
  parallelWorkersPerGather = lib.min 4 (lib.max 1 (threads / 3));
in
{
  options.${namespace}.services.postgresql = {
    enable = mkEnableOption "Whether or not to manage postgresql.";
    package = mkPackageOption pkgs "postgresql_17" { };
  };

  config = mkIf cfg.enable {
    ${namespace}.system = {
      impermanence.directories = [ "/var/lib/postgresql" ];
      fs.btrfs.nocowPaths = [ "/var/lib/postgresql" ];
    };

    services.postgresql = {
      enable = true;
      package = lib.mkForce cfg.package;
      dataDir = lib.mkForce "/var/lib/postgresql/${cfg.package.psqlSchema}";
      authentication = lib.mkForce ''
        local all all trust
      '';
      settings = {
        shared_buffers = "${toString sharedBuffersMiB}MB";
        effective_cache_size = "${toString effectiveCacheSizeMiB}MB";
        maintenance_work_mem = "512MB";
        work_mem = "16MB";

        max_worker_processes = threads;
        max_parallel_workers = threads;
        max_parallel_workers_per_gather = parallelWorkersPerGather;

        effective_io_concurrency = 200;

        random_page_cost = 1.1;
        max_wal_size = "2GB";
        min_wal_size = "1GB";
        checkpoint_completion_target = 0.9;
        checkpoint_timeout = "15min";

        commit_delay = 1000;
        commit_siblings = 5;

        wal_compression = "zstd";
        default_toast_compression = "lz4";
        track_io_timing = true;
      };
    };

    environment.systemPackages = [
      (
        let
          newPostgres = pkgs.postgresql_17.withPackages (_pp: [
            # _pp.plv8
          ]);
          cfg = config.services.postgresql;
        in
        pkgs.writeScriptBin "upgrade-pg-cluster" ''
          set -eux
          # XXX it's perhaps advisable to stop all services that depend on postgresql
          systemctl stop postgresql

          export NEWDATA="/var/lib/postgresql/${newPostgres.psqlSchema}"
          export NEWBIN="${newPostgres}/bin"

          export OLDDATA="${cfg.dataDir}"
          export OLDBIN="${cfg.finalPackage}/bin"

          install -d -m 0700 -o postgres -g postgres "$NEWDATA"
          cd "$NEWDATA"
          sudo -u postgres "$NEWBIN/initdb" -D "$NEWDATA" ${lib.escapeShellArgs cfg.initdbArgs}

          sudo -u postgres "$NEWBIN/pg_upgrade" \
            --old-datadir "$OLDDATA" --new-datadir "$NEWDATA" \
            --old-bindir "$OLDBIN" --new-bindir "$NEWBIN" \
            "$@"
        ''
      )
    ];
  };
}
