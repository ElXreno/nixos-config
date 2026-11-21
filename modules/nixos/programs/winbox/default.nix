{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.winbox;
in
{
  options.${namespace}.programs.winbox = {
    enable = mkEnableOption "managing winbox";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.networking.firewall.public.winbox = {
      upstream = "nixos/modules/programs/winbox.nix";
      udp = [
        5678
        20561
      ];
      udpRanges = [
        {
          from = 40000;
          to = 50000;
        }
      ];
    };

    programs.winbox = {
      enable = true;
      openFirewall = true;
    };
  };
}
