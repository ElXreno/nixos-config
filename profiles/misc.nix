{
  config,
  pkgs,
  lib,
  ...
}:
{
  time.timeZone = "Europe/Minsk";

  i18n.defaultLocale = lib.mkDefault "en_US.UTF-8";
  console = {
    # font = lib.mkDefault "Lat2-Terminus16";
    keyMap = lib.mkDefault "us";
  };

  documentation = lib.mkIf config.deviceSpecific.isServer {
    enable = false;
    nixos.enable = false;
    man.enable = false;
    info.enable = false;
    doc.enable = false;
  };

  home-manager = {
    backupFileExtension = ".bak";
    users.elxreno = {
      systemd.user.startServices = "sd-switch";
      xdg.mimeApps.enable = true;
      home = {
        inherit (config.system) stateVersion;
        sessionPath = [ "${config.users.users.elxreno.home}/bin" ];
        sessionVariables = lib.mkMerge [
          {
            EDITOR = "hx";
            _JAVA_OPTIONS = "-Dawt.useSystemAAFontSettings=lcd";
            DOTNET_CLI_TELEMETRY_OPTOUT = 1;
          }
          (lib.mkIf config.deviceSpecific.isLaptop {
            NIXPKGS = "${config.users.users.elxreno.home}/projects/repos/github.com/NixOS/nixpkgs";
            # go brrr
            GOROOT = "${pkgs.go}/share/go";
            GOPATH = "${config.users.users.elxreno.home}/.go";
          })
        ];
      };
    };
  };

  services.irqbalance.enable = true;

  virtualisation.podman = {
    enable = true;
    dockerSocket.enable = true;
  };
  hardware.nvidia-container-toolkit.enable = config.hardware.nvidia.enabled;

  zramSwap = {
    enable = lib.mkDefault true;
    algorithm = "zstd";
    memoryPercent = 300;
  };

  # Not in use anyway
  systemd.coredump.enable = false;
}
