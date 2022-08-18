{ config, pkgs, lib, ... }:
{
  time.timeZone = lib.mkDefault "Europe/Minsk";

  i18n.defaultLocale = lib.mkDefault "en_US.UTF-8";
  console = {
    font = lib.mkDefault "Lat2-Terminus16";
    keyMap = lib.mkDefault "us";
  };

  documentation = lib.mkIf config.deviceSpecific.isServer {
    enable = false;
    nixos.enable = false;
    man.enable = false;
    info.enable = false;
    doc.enable = false;
  };

  home-manager.users.elxreno = {
    xdg.mimeApps.enable = true;
    home = {
      inherit (config.system) stateVersion;
      sessionPath = [ "${config.users.users.elxreno.home}/bin" ];
      sessionVariables = lib.mkMerge [
        {
          EDITOR = "vim";
          _JAVA_OPTIONS = "-Dawt.useSystemAAFontSettings=lcd";
          DOTNET_CLI_TELEMETRY_OPTOUT = true;
        }
        (
          lib.mkIf config.deviceSpecific.isLaptop {
            NIXPKGS = "${config.users.users.elxreno.home}/projects/repos/github.com/NixOS/nixpkgs";
            # go brrr
            GOROOT = "${pkgs.go}/share/go";
            GOPATH = "${config.users.users.elxreno.home}/.go";
          }
        )
      ];
    };
  };

  services.irqbalance.enable = true;

  virtualisation = {
    podman = {
      enable = true;
      # dockerCompat = true;
    };
    # docker.enable = true;
  };

  zramSwap = {
    enable = true;
    # zstd is bad for high-load memory tasks
    algorithm = "lzo-rle";
    memoryPercent = 150;
  };
}
