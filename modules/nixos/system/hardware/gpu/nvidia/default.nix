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
    ;
  cfg = config.${namespace}.system.hardware.gpu.nvidia;

  busIDType = lib.types.strMatching "([[:print:]]+:[0-9]{1,3}(@[0-9]{1,10})?:[0-9]{1,2}:[0-9])?";
in
{
  options.${namespace}.system.hardware.gpu.nvidia = {
    enable = mkEnableOption "Whether or not to manage nvidia stuff.";
    package = mkPackageOption config.boot.kernelPackages.nvidiaPackages "latest" { };

    modesetting.enable = mkEnableOption "Whether to enable kernel modesetting.";

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
        powerManagement.enable = true;
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

    systemd.services = {
      nvidia_oc = mkIf cfg.overclock.enable {
        description = "NVIDIA Overclocking Service";
        after = [ "graphical.target" ];
        wantedBy = [ "graphical.target" ];

        serviceConfig = {
          ExecStart = "${pkgs.nvidia_oc}/bin/nvidia_oc set --index 0 --freq-offset 120 --min-clock 210 --max-clock 2670 --mem-offset 200";
          User = "root";
          Restart = "on-failure";
        };
      };

      nvidia-powerd.serviceConfig.Restart = mkIf cfg.dynamicBoost.enable "always";
    };
  };
}
