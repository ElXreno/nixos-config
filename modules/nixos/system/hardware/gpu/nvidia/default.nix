{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkPackageOption
    mkOption
    mkForce
    ;
  cfg = config.${namespace}.system.hardware.gpu.nvidia;

  busIDType = lib.types.strMatching "([[:print:]]+:[0-9]{1,3}(@[0-9]{1,10})?:[0-9]{1,2}:[0-9])?";
in
{
  options.${namespace}.system.hardware.gpu.nvidia = {
    enable = mkEnableOption "Whether or not to manage nvidia stuff.";
    enableBatterySaverSpecialisation = mkEnableOption "Whether to enable battery saver specialisation.";
    package = mkPackageOption config.boot.kernelPackages.nvidiaPackages "latest" { };

    modesetting.enable = mkEnableOption "Whether to enable kernel modesetting.";

    powerManagement = {
      enable = mkEnableOption "Whether to enable power management." // {
        default = true;
      };
      finegrained = mkEnableOption "Whether to enable fine-grained power management.";
    };

    dynamicBoost.enable = mkEnableOption "Whether to enable nvidia-powerd.";
    overclock.enable = mkEnableOption "Whether to overclock NVIDIA GPU.";

    prime = {
      enable = mkEnableOption "Whether to enable Prime support.";
      amdgpuBusId = mkOption {
        type = busIDType;
        default = "";
      };
      intelBusId = mkOption {
        type = busIDType;
        default = "";
      };
      nvidiaBusId = mkOption {
        type = busIDType;
        default = "";
      };
    };

  };

  config = mkIf cfg.enable {
    environment.variables = {
      # Let's ensure that shaders cache are used & also cleanup cache clean up
      __GL_SHADER_DISK_CACHE = "1";
      __GL_SHADER_DISK_CACHE_PATH = "$HOME/.nv_shader_cache";
      __GL_SHADER_DISK_CACHE_SKIP_CLEANUP = "1";
    };

    services.xserver.videoDrivers = [
      "nvidia"
    ];

    hardware = {
      nvidia = {
        inherit (cfg) package;
        open = cfg.package ? open && cfg.package ? firmware;
        modesetting.enable = cfg.modesetting.enable;
        dynamicBoost.enable = cfg.dynamicBoost.enable;
        powerManagement = {
          inherit (cfg.powerManagement) enable;
          inherit (cfg.powerManagement) finegrained;
        };
        # nvidiaPersistenced = true;

        prime = mkIf cfg.prime.enable {
          inherit (cfg.prime) amdgpuBusId intelBusId nvidiaBusId;

          offload = {
            enable = true;
            enableOffloadCmd = true;
          };
        };
      };
    };

    specialisation = mkIf cfg.enableBatterySaverSpecialisation {
      battery-saver.configuration = {
        system.nixos.tags = [ "battery-saver" ];
        boot = {
          extraModprobeConfig = ''
            blacklist nouveau
            options nouveau modeset=0
          '';
          blacklistedKernelModules = [
            "nouveau"
            "nvidia"
            "nvidia_drm"
            "nvidia_modeset"
          ];
        };

        services.udev.extraRules = ''
          # Remove NVIDIA USB xHCI Host Controller devices, if present
          ACTION=="add", SUBSYSTEM=="pci", ATTR{vendor}=="0x10de", ATTR{class}=="0x0c0330", ATTR{power/control}="auto", ATTR{remove}="1"

          # Remove NVIDIA USB Type-C UCSI devices, if present
          ACTION=="add", SUBSYSTEM=="pci", ATTR{vendor}=="0x10de", ATTR{class}=="0x0c8000", ATTR{power/control}="auto", ATTR{remove}="1"

          # Remove NVIDIA Audio devices, if present
          ACTION=="add", SUBSYSTEM=="pci", ATTR{vendor}=="0x10de", ATTR{class}=="0x040300", ATTR{power/control}="auto", ATTR{remove}="1"

          # Remove NVIDIA VGA/3D controller devices
          ACTION=="add", SUBSYSTEM=="pci", ATTR{vendor}=="0x10de", ATTR{class}=="0x03[0-9]*", ATTR{power/control}="auto", ATTR{remove}="1"
        '';

        hardware.nvidia = {
          prime.offload = {
            enable = mkForce false;
            enableOffloadCmd = mkForce false;
          };
          dynamicBoost.enable = mkForce false;
          powerManagement = {
            enable = mkForce false;
            finegrained = mkForce false;
          };
        };

        systemd.services.nvidia_oc = mkForce { };
      };
    };

    systemd.services = {
      nvidia_oc = mkIf cfg.overclock.enable {
        description = "NVIDIA Overclocking Service";
        after = [ "graphical.target" ];
        wantedBy = [ "graphical.target" ];

        serviceConfig = {
          ExecStart = "${pkgs.nvidia_oc}/bin/nvidia_oc set --index 0 --freq-offset 140 --min-clock 0 --max-clock 2760 --mem-offset 1000";
          User = "root";
          Restart = "on-failure";
        };
      };

      nvidia-powerd.serviceConfig.Restart = mkIf cfg.dynamicBoost.enable "always";
    };
  };
}
