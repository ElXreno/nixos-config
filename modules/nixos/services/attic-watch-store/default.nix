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
    escapeShellArg
    ;
  cfg = config.${namespace}.services.attic-watch-store;
  cacheRef = "${cfg.serverName}:${cfg.cacheName}";
in
{
  options.${namespace}.services.attic-watch-store = {
    enable = mkEnableOption "Whether to watch the local Nix store and push new paths to the attic cache.";

    serverName = mkOption {
      type = types.str;
      default = "elxreno";
      description = "Local alias for the attic server (the first arg to `attic login`).";
    };

    serverUrl = mkOption {
      type = types.str;
      default = "https://cache.elxreno.com";
      description = "URL of the attic server.";
    };

    cacheName = mkOption {
      type = types.str;
      default = "common";
      description = "Name of the cache on the attic server.";
    };
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.attic-watch-store = {
      dependencies = [ "attic-jwt-key" ];
      files.token = {
        secret = true;
        restartUnits = [ "attic-watch-store.service" ];
      };

      # Rotate weekly.
      rotateDays = 7;

      runtimeInputs = [ config.${namespace}.services.atticd.mintToken ];

      script = ''
        attic-mint-token "$in/attic-jwt-key/key-base64" \
          --sub ${escapeShellArg "watch-store@${config.clan.core.settings.machine.name}"} \
          --validity "14 days" \
          --pull ${escapeShellArg cfg.cacheName} \
          --push ${escapeShellArg cfg.cacheName} \
          > "$out/token"
      '';
    };

    systemd.services.attic-watch-store = {
      description = "Attic watch-store: push new Nix store paths to ${cacheRef}";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      environment.HOME = "/run/attic-watch-store";
      path = [ pkgs.attic-client ];
      serviceConfig = {
        DynamicUser = true;
        MemoryHigh = "5%";
        MemoryMax = "10%";
        LoadCredential = "auth-token:${config.clan.core.vars.generators.attic-watch-store.files.token.path}";
        RuntimeDirectory = "attic-watch-store";
        Restart = "on-failure";
        RestartSec = "10s";
      };
      script = ''
        set -eu -o pipefail
        ATTIC_TOKEN=$(< "$CREDENTIALS_DIRECTORY/auth-token")
        attic login ${escapeShellArg cfg.serverName} ${escapeShellArg cfg.serverUrl} "$ATTIC_TOKEN"
        exec attic watch-store ${escapeShellArg cacheRef}
      '';
    };
  };
}
