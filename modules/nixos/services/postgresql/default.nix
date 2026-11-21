{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    attrNames
    concatMapStringsSep
    concatStringsSep
    escapeShellArg
    filterAttrs
    mapAttrsToList
    mkIf
    mkEnableOption
    mkOption
    mkPackageOption
    optional
    types
    ;
  cfg = config.${namespace}.services.postgresql;

  plainDatabases = filterAttrs (_: db: db.locale == null) cfg.databases;
  localeDatabases = filterAttrs (_: db: db.locale != null) cfg.databases;
  tunedDatabases = filterAttrs (_: db: db.extensions != [ ] || db.settings != { }) cfg.databases;

  psql = "${config.services.postgresql.finalPackage}/bin/psql -X";

  createLocaleDatabase = name: db: ''
    if [ "$(${psql} -tAc "SELECT 1 FROM pg_database WHERE datname = '${name}'")" != "1" ]; then
      createdb \
        --owner=${escapeShellArg db.owner} \
        --template=template0 \
        --encoding=UTF8 \
        --locale=${escapeShellArg db.locale} \
        ${escapeShellArg name}
    fi
  '';

  tuneDatabase = name: db: ''
    ${concatMapStringsSep "\n" (ext: ''
      ${psql} -d ${escapeShellArg name} -tAc ${escapeShellArg "ALTER EXTENSION ${ext} UPDATE;"} || true
      ${psql} -d ${escapeShellArg name} -tAc ${escapeShellArg "CREATE EXTENSION IF NOT EXISTS ${ext};"}
    '') db.extensions}
    ${concatStringsSep "\n" (
      mapAttrsToList (
        key: value: "${psql} -tAc ${escapeShellArg "ALTER DATABASE \"${name}\" SET ${key} = ${value};"}"
      ) db.settings
    )}
  '';
  inherit (config.${namespace}) facts;

  inherit (facts.memory) totalMiB;
  inherit (facts.cpu) threads;

  # conservative ratios for shared-tenant hosts like BIMBA
  sharedBuffersMiB = lib.max 128 (totalMiB / 16);
  effectiveCacheSizeMiB = totalMiB * 3 / 16;
  parallelWorkersPerGather = lib.min 4 (lib.max 1 (threads / 3));

  dataDirOnZfs = (config.fileSystems."/var/lib/postgresql".fsType or null) == "zfs";
in
{
  options.${namespace}.services.postgresql = {
    enable = mkEnableOption "managing postgresql";
    package = mkPackageOption pkgs "postgresql_17" { };
    databases = mkOption {
      type = types.attrsOf (
        types.submodule (
          { name, ... }:
          {
            options = {
              owner = mkOption {
                type = types.str;
                default = name;
                description = "Role that owns the database.";
              };
              locale = mkOption {
                type = types.nullOr types.str;
                default = null;
                description = "Create the database from template0 with this locale instead of the cluster default.";
              };
              extensions = mkOption {
                type = types.listOf types.str;
                default = [ ];
                description = "Extensions created (and updated) in the database by the superuser.";
              };
              settings = mkOption {
                type = types.attrsOf types.str;
                default = { };
                description = "Per-database parameters applied with `ALTER DATABASE ... SET`.";
              };
              backup = mkEnableOption "dumping the database into this machine's kopia repository";
            };
          }
        )
      );
      default = { };
      description = "Databases to create, with their owners, locale, extensions and backup.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      system = mkIf (!dataDirOnZfs) {
        impermanence.directories = [ "/var/lib/postgresql" ];
        fs.btrfs.nocowPaths = [ "/var/lib/postgresql" ];
      };

      services.kopia.backup = map (db: {
        path = "/postgresql/${db}";
        fileName = "${db}.sql";
        command = "${pkgs.util-linux}/bin/runuser -u postgres -- ${config.services.postgresql.finalPackage}/bin/pg_dump --format=plain ${escapeShellArg db}";
      }) (attrNames (filterAttrs (_: db: db.backup) cfg.databases));
    };

    systemd.services.postgresql-setup.preStart = ''
      psql -d postgres -qc 'ALTER DATABASE template1 REFRESH COLLATION VERSION'
    '';

    systemd.services.postgresql-collation-refresh = {
      description = "Reindex PostgreSQL databases whose collation version changed";
      wantedBy = [ "multi-user.target" ];
      after = [ "postgresql.target" ];
      requires = [ "postgresql.target" ];
      path = [ config.services.postgresql.finalPackage ];
      serviceConfig = {
        Type = "exec";
        User = "postgres";
      };
      script = ''
        psql -X -Atq -d postgres -c "
          select datname from pg_database
          where datallowconn and datcollversion is not null
            and datcollversion is distinct from pg_database_collation_actual_version(oid)
        " | while IFS= read -r database; do
          echo "reindexing $database"
          psql -X -q -v ON_ERROR_STOP=1 -d "$database" <<'SQL'
        REINDEX DATABASE;
        select format('ALTER DATABASE %I REFRESH COLLATION VERSION', current_database()) \gexec
        SQL
        done
      '';
    };

    systemd.services.postgresql-databases-init = mkIf (localeDatabases != { }) {
      description = "Create PostgreSQL databases with a non-default locale";
      after = [ "postgresql.target" ];
      requires = [ "postgresql.target" ];
      wantedBy = [ "multi-user.target" ];
      path = [ config.services.postgresql.package ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        User = "postgres";
        Group = "postgres";
      };
      script = ''
        set -eu
        ${concatStringsSep "\n" (mapAttrsToList createLocaleDatabase localeDatabases)}
      '';
    };

    systemd.services.postgresql-databases-tune = mkIf (tunedDatabases != { }) {
      description = "Create PostgreSQL extensions and apply per-database settings";
      after = [
        "postgresql-setup.service"
      ]
      ++ optional (localeDatabases != { }) "postgresql-databases-init.service";
      requires = [ "postgresql-setup.service" ];
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        User = "postgres";
        Group = "postgres";
      };
      script = concatStringsSep "\n" (mapAttrsToList tuneDatabase tunedDatabases);
    };

    services.postgresql = {
      enable = true;
      ensureDatabases = attrNames plainDatabases;
      ensureUsers = mapAttrsToList (name: db: {
        name = db.owner;
        ensureDBOwnership = db.locale == null && db.owner == name;
      }) cfg.databases;
      inherit (cfg) package;
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
        max_wal_size = "8GB";
        min_wal_size = "1GB";
        wal_buffers = "64MB";
        checkpoint_completion_target = 0.9;
        checkpoint_timeout = "15min";

        commit_delay = 1000;
        commit_siblings = 5;

        wal_compression = "zstd";
        default_toast_compression = "lz4";
        track_io_timing = true;
      }
      // lib.optionalAttrs dataDirOnZfs {
        full_page_writes = false;
        wal_init_zero = false;
        wal_recycle = false;
      };
    };
  };
}
