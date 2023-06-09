{ config, inputs, pkgs, lib, ... }:

{
  specialisation = {
    k8s.configuration = {
      imports = [
        (import inputs.self.nixosProfiles.k8s-master {
          inherit pkgs lib;
          kubeMasterHostname = config.device;
          kubeMasterIP = "100.120.26.5";
        })
      ];

      networking.firewall.allowedTCPPorts = [ 3000 ];
      networking.firewall.allowedUDPPorts = [ 3000 ];
    };
  };
}
