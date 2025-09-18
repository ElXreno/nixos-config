{ config, pkgs, lib, ... }:
{
  services.postgresql = {
    enable = true;
    package = pkgs.postgresql_17;
    authentication = lib.mkForce ''
      local all all trust
    '';
    settings = {
      shared_buffers = "1536MB";
      effective_cache_size = "4GB";
      maintenance_work_mem = "512MB";
      work_mem = "16MB";

      random_page_cost = 1.1;
      max_wal_size = "2GB";
      min_wal_size = "1GB";
      checkpoint_completion_target = 0.9;
      checkpoint_timeout = "15min";

      commit_delay = 1000;
      commit_siblings = 5;
    };
  };

  environment.systemPackages = [
    (
      let
        newPostgres = pkgs.postgresql_17.withPackages (pp: [
          # pp.plv8
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
}
