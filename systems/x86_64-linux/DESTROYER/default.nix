{
  inputs,
  namespace,
  ...
}:

{
  imports = [
    ./disko-config.nix
    ./wireguard.nix

    inputs.nixos-facter-modules.nixosModules.facter
    { config.facter.reportPath = ./facter.json; }
  ];

  ${namespace} = {
    roles.server.enable = true;

    system = {
      boot.legacy = {
        enable = true;
        setupDevice = false;
      };

      hardware = {
        cpu.manufacturer = "amd";
      };
    };

    services = {
      atticd.enable = true;
      # xray.server.enable = true;
      sing-box.server.enable = true;
      nginx.enable = true;
      factorio.enable = true;
    };
  };

  services.qemuGuest.enable = true;

  security.sudo.wheelNeedsPassword = false;

  networking.useNetworkd = true;
  networking.enableIPv6 = true;
  systemd.network = {
    enable = true;
    networks."10-ens3" = {
      matchConfig.Name = "ens3";
      address = [
        "74.119.195.240/24"
        "2a09:7c44:0:3e::/64"
      ];
      routes = [
        { Gateway = "74.119.195.1"; }
        { Gateway = "2a09:7c44::1"; }
      ];
      linkConfig.RequiredForOnline = "routable";
    };
  };

  security.pam.loginLimits = [
    {
      domain = "*";
      item = "nofile";
      type = "-";
      value = "32768";
    }
  ];

  services.bpftune.enable = true;
  boot.kernelParams = [
    "virtio_net.napi_weight=64"

    "processor.max_cstate=1"
    "idle=nomwait"
    "preempt=voluntary"

    "iommu.strict=0"
    "iommu.passthrough=1"
  ];

  system.stateVersion = "25.05";
}
