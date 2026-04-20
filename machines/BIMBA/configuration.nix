{
  namespace,
  pkgs,
  ...
}:

{
  ${namespace} = {
    roles.server.enable = true;

    system = {
      boot = {
        uefi.enable = true;

        kernel = {
          packages = pkgs.linuxPackages_xanmod_latest;
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
      matrix.synapse.enable = true;
      nginx = {
        enable = true;
        enableDefaultVhost = true;
      };

      ripe-atlas.enable = true;
    };
  };

  # Don't touch hardware.graphics.enable
  hardware.facter.detected.graphics.enable = false;

  services.qemuGuest.enable = true;

  networking.useNetworkd = true;
  networking.enableIPv6 = true;
  systemd.network = {
    enable = true;
    networks."10-ens3" = {
      matchConfig.Name = "ens3";
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

  nix.settings.system-features = [ "gccarch-znver4" ];
}
