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
    ;
  cfg = config.${namespace}.user.elxreno;
in
{
  options.${namespace}.user.elxreno = {
    enable = mkEnableOption "Whether to configure ElXreno user." // {
      default = config.home.username == "elxreno";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.programs.firefox = {
      extensions.packages = with pkgs.firefox-addons; [ keepassxc-browser ];
    };
  };
}
