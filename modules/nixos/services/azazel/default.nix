{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.azazel;
in
{
  options.${namespace}.services.azazel = {
    enable = mkEnableOption "the PostgreSQL database of the azazel scraper";
  };

  config = mkIf cfg.enable {
    ${namespace}.services.postgresql = {
      enable = true;
      databases.azazel_lowcost = {
        extensions = [
          "timescaledb"
          "vector"
          "pg_trgm"
        ];
        settings.synchronous_commit = "off";
      };
    };

    services.postgresql = {
      extensions =
        ps: with ps; [
          timescaledb
          pgvector
        ];
      settings.shared_preload_libraries = "timescaledb";
      authentication = "local azazel_lowcost azazel_lowcost trust";
    };
  };
}
