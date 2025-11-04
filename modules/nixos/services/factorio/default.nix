{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.factorio;
in
{
  options.${namespace}.services.factorio = {
    enable = mkEnableOption "Whether or not to manage factorio.";
  };

  config = mkIf cfg.enable {
    services.factorio = {
      enable = true;
      package = pkgs.factorio-headless.override {
        versionsJson = ./factorio-versions.json;
      };
      admins = [ "elxreno" ];
      autosave-interval = 5;
      game-name = "abrakadabra";
      description = "Space Age 2.000000000";
      lan = true;
      port = 34197;
    };
  };
}
