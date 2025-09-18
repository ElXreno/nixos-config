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

  services.irqbalance.enable = config.device != "KURWA";
  systemd.services.irqbalance.serviceConfig.ProtectKernelTunables = "no";

  systemd.services."irq-pin" = lib.mkIf (config.device == "KURWA") {
    description = "Pin selected IRQs to specific CPU masks";
    wantedBy = [ "multi-user.target" ];
    after = [ "multi-user.target" ];
    serviceConfig = {
      Type = "oneshot";
      CapabilityBoundingSet = [ "CAP_SETPCAP" ];
      ExecStart =
        let
          pin-irq = pkgs.writeShellScript "pin-irq" ''
            set -eu

            hex_ui="0003"
            hex_noise="FF00"

            ${lib.getExe pkgs.gawk} -v ui="$hex_ui" -v noise="$hex_noise" '
              /^[[:space:]]*[0-9]+:/ {
                irq = $1
                sub(":", "", irq)

                is_ui = (index($0, "amdgpu") > 0 || index($0, "nvidia") > 0)
                mask  = is_ui ? ui : noise
                label = is_ui ? "UI" : "noise"

                system("echo irq " irq " - " label)

                f = "/proc/irq/" irq "/smp_affinity"
                if (system("[ -w " f " ]") == 0) {
                  system("echo " mask " > " f)
                }
              }
            ' /proc/interrupts
          '';
        in
        [
          pin-irq
        ];
    };
  };

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
