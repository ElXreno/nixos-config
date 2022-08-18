{ config, pkgs, ... }:

{
  networking.firewall.allowedTCPPorts = [ 25565 8100 ];
  networking.firewall.allowedUDPPorts = [ 25565 ];

  home-manager.users.elxreno = {
    home = {
      packages = with pkgs; [
        adoptopenjdk-jre-openj9-bin-16
      ];
    };
  };
}
