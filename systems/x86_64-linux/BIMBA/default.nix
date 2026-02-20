{
  inputs,
  namespace,
  pkgs,
  ...
}:

{
  imports = [
    ./disko-config.nix

    inputs.nixos-facter-modules.nixosModules.facter
    { config.facter.reportPath = ./facter.json; }
  ];

  ${namespace} = {
    roles.server.enable = true;

    system = {
      boot = {
        uefi.enable = true;

        kernel = {
          packages = pkgs.linuxPackages_xanmod_latest;
          optimizations = {
            enable = true;
            isa = 3;
          };
        };
      };

      hardware = {
        cpu.manufacturer = "amd";
      };

      virtualisation.containers.minecraft.atm10 = {
        enable = true;
      };
    };

    services = {
      atticd.enable = true;
      nginx = {
        enable = true;
        enableDefaultVhost = true;
      };

      ripe-atlas.enable = true;
    };
  };

  # Don't touch hardware.graphics.enable
  facter.detected.graphics.enable = false;

  services.qemuGuest.enable = true;

  security.sudo.wheelNeedsPassword = false;

  networking.useNetworkd = true;
  networking.enableIPv6 = true;
  systemd.network = {
    enable = true;
    networks."10-eth0" = {
      matchConfig.Name = "eth0";
      address = [
        "159.195.56.52/22"
        "2a0a:4cc0:c1:e9be::/64"
      ];
      routes = [
        { Gateway = "159.195.56.1"; }
        { Gateway = "fe80::1"; }
      ];
      linkConfig.RequiredForOnline = "routable";
    };
  };

  services.bpftune.enable = true;

  nix.settings.system-features = [ "gccarch-znver4" ];

  system.stateVersion = "25.05";
}
