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
    enable = mkEnableOption "managing wireshark";
  };

  config = mkIf cfg.enable {
    programs.wireshark = {
      enable = true;
      package = pkgs.wireshark;
      dumpcap.enable = true;
      usbmon.enable = true;
    };

    users.users.${config.${namespace}.primaryUser}.extraGroups = [ "wireshark" ];
  };
}
