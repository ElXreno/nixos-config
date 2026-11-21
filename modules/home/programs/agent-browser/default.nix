{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.agent-browser;
in
{
  options.${namespace}.programs.agent-browser = {
    enable = mkEnableOption "managing agent-browser";
  };

  config = mkIf cfg.enable {
    home.packages = [ pkgs.agent-browser ];
  };
}
