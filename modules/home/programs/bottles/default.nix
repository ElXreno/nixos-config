{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.bottles;
in
{
  options.${namespace}.programs.bottles = {
    enable = mkEnableOption "Whether or not to manage bottles.";
  };

  config = mkIf cfg.enable {
    home.packages = with pkgs; [ bottles ];
  };
}
