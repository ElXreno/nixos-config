{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption mkMerge;
  cfg = config.${namespace}.system.boot;
in
{
  options.${namespace}.system.boot = {
    enable = mkEnableOption "managing boot stuff";
    setupBootloader = mkEnableOption "setting up bootloader settings" // {
      default = true;
    };
    interactive = mkEnableOption "a boot menu timeout, the splash screen and NTFS support" // {
      default = true;
    };
    lowLatency = mkEnableOption "full preemption with the soft and NMI watchdogs off";
    uefi = {
      secureBoot = mkEnableOption "using secure boot";
      fallbackEsp = lib.mkOption {
        type = with lib.types; nullOr str;
        default = null;
        description = "Mountpoint of a secondary ESP to sync the bootloader to on every install.";
      };
    };
  };

  config = mkIf cfg.enable {
    boot = {
      loader = mkIf cfg.setupBootloader (mkMerge [
        {
          timeout = if cfg.interactive then 3 else 0;
          efi.canTouchEfiVariables = true;
          grub.enable = false;
          systemd-boot = {
            enable = !cfg.uefi.secureBoot;
            configurationLimit = 10;
          };
        }

        (mkIf (cfg.uefi.fallbackEsp != null) {
          systemd-boot.extraInstallCommands = ''
            if ${pkgs.util-linux}/bin/mountpoint -q ${cfg.uefi.fallbackEsp}; then
              ${lib.getExe pkgs.rsync} -r --times --modify-window=1 --delete \
                --exclude=loader/random-seed \
                ${config.boot.loader.efi.efiSysMountPoint}/ ${cfg.uefi.fallbackEsp}/
            else
              echo "fallback ESP ${cfg.uefi.fallbackEsp} not mounted, skipping sync" >&2
            fi
          '';
        })
      ]);

      lanzaboote = {
        enable = cfg.uefi.secureBoot;
        pkiBundle = "/var/lib/sbctl";
      };

      kernelParams = [
        "nohibernate"
        "mitigations=off"
      ]
      ++ lib.optionals cfg.lowLatency [
        "preempt=full"
        "nowatchdog"
        "nmi_watchdog=0"
      ];

      supportedFilesystems = mkIf cfg.interactive [ "ntfs" ];

      initrd = {
        compressor = "zstd";
        compressorArgs = [
          "-19"
          "-T0"
        ];
      };

      plymouth.enable = cfg.interactive;
    };
  };
}
