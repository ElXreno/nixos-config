{
  networking.firewall = {
    allowedTCPPorts = [ 22000 ]; # TCP based sync protocol traffic
    allowedUDPPorts = [ 22000 21027 ]; # QUIC based sync protocol traffic & for discovery broadcasts
  };

  home-manager.users.elxreno.services.syncthing.enable = true;
}
