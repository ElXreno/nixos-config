{
  inputs,
  ...
}:

{
  imports = [
    inputs.nixos-facter-modules.nixosModules.facter
    { config.facter.reportPath = ./facter.json; }

    inputs.disko.nixosModules.disko
    inputs.self.diskoConfigurations.destroyer
    inputs.self.nixosRoles.server
    inputs.self.nixosProfiles.services.tailscale
    inputs.self.nixosProfiles.services.atticd

    ./wireguard.nix
  ];

  deviceSpecific.usesCustomBootloader = true;
  boot.loader.grub.enable = true;

  services.qemuGuest.enable = true;

  security.sudo.wheelNeedsPassword = false;

  networking.useDHCP = false;
  networking.useNetworkd = true;
  networking.enableIPv6 = false;
  systemd.network = {
    enable = true;
    networks = {
      "ens3" = {
        matchConfig.Name = "ens3";
        address = [ "74.119.195.240/24" ];
        routes = [
          { Gateway = "74.119.195.1"; }
        ];
        linkConfig.RequiredForOnline = "routable";
      };
    };
  };

  services.bpftune.enable = true;

  system.stateVersion = "25.05";
}
