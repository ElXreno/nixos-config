{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.wireshark;
in
{
  options.${namespace}.programs.wireshark = {
    enable = mkEnableOption "Whether or not to manage wireshark.";
  };

  config = mkIf cfg.enable {
    programs.wireshark = {
      enable = true;
      package = pkgs.wireshark;
      dumpcap.enable = true;
      usbmon.enable = true;
    };

    ${namespace}.user.elxreno.extraGroups = [ "wireshark" ];
  };
}
