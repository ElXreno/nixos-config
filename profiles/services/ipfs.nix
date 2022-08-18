{ config, lib, ... }:

{
  networking.firewall.allowedTCPPorts = [ 4001 ];
  networking.firewall.allowedUDPPorts = [ 4001 ];

  boot.kernel.sysctl."net.core.rmem_max" = 2500000;

  services.ipfs = {
    enable = true;
    enableGC = true;
    localDiscovery = false;
    apiAddress = "/ip4/127.0.0.1/tcp/5001";
  };

  systemd.services.ipfs = {
    serviceConfig = {
      MemoryMax = lib.mkIf config.services.minecraft-server.enable "256M";
      MemorySwapMax = "infinity";
      CPUSchedulingPolicy = "idle";
      CPUWeight = 12;
      IOSchedulingClass = "idle";
      IOWeight = 10;
      Nice = 19;
    };
  };
}
