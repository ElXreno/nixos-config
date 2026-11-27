{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.camoufox-cli;
in
{
  options.${namespace}.programs.camoufox-cli = {
    enable = mkEnableOption "managing camoufox-cli";
  };

  config = mkIf cfg.enable {
    home.packages = [ pkgs.${namespace}.camoufox-cli ];
  };
}
