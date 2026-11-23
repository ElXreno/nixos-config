{
  config,
  namespace,
  ...
}:

{
  imports = [ ./llm.nix ];

  ${namespace} = {
    nixpkgs.cudaSupport = true;

    system = {
      boot.uefi.fallbackEsp = "/boot-fallback";

      impermanence = {
        enable = true;
        directories = [ "/var/lib/bert-archive" ];
      };

      ccache = {
        enable = true;
        owner = config.${namespace}.primaryUser;
        group = "users";
        aliases = [ "/nix/ccache" ];
      };

      nix.buildDir = "/mnt/scratch/nix-builds";

      networking = {
        tunnels.airvpn = {
          mtu = 1432;
          # Piautos, entry 3
          endpoints = [ "134.19.178.169:1637" ];
        };
        routes.cgroups = [ "warp" ];
        firewall.public.torrents = {
          tcp = [ 56660 ];
          udp = [ 56660 ];
        };
      };

      hardware = {
        gpu.nvidia = {
          eglWayland2.enable = false;
          overclock = {
            enable = true;
            freqOffset = 250;
          };

          moduleParams.NVreg_RegistryDwords = "RmEnableAggressiveVblank=1";
        };

        peripherals = {
          lamzu.enable = true;
          zaopin.enable = true;
          silakka54.enable = true;
          libinput.neverDebounce = true;
        };
        mic.fifine-am8.enable = true;
      };

      fs.zfs = {
        hostId = "e7e00058";
        autoSnapshot.filesystems = {
          "rpool/datalake" = true;
          "rpool/home-persist" = true;
          "rpool/root-persist<" = true;
        };
        autoSnapshot.retention."rpool/root-persist/postgres" = "4x15m(keep=all) | 4x1h | 1x1d";
      };

      virtualisation = {
        libvirtd.enable = true;
        podman.enable = true;
      };
    };

    services = {
      azazel.enable = true;
      kubo = {
        enable = true;
        forwardedPort = 2769;
      };
      niks3.push.enable = true;
      nix-farm = {
        enable = true;
        acOnly = true;
        pauseFor = [
          "freetoken-ornith-backend"
          "freetoken-ornith35-backend"
          "strata-qwen-backend"
          "strata-swift-backend"
        ];
      };

      flydigictl.enable = true;

      sunshine = {
        enable = true;
        encoder = "auto";
        virtualOutput.enable = true;
      };

      zerofs.volumes.scratch = {
        metricsPort = 9812;
        lockPort = 9813;
        archive = {
          enable = true;
          type = "session-dirs";
          source = "/mnt/scratch/claude-code";
        };
      };
    };

    programs = {
      allmodels.enable = true;
      obs-studio.enable = true;
      steam.enable = true;
    };

    home-manager.syncthing.randomPortIncrement = 42;
  };

  services = {
    rk-m87-sync.enable = true;
  };

  boot.binfmt.emulatedSystems = [ "aarch64-linux" ];

  boot.kernelParams = [ "pstore.kmsg_bytes=65536" ];

  boot.kernel.sysctl = {
    "kernel.hardlockup_all_cpu_backtrace" = 1;
    "kernel.hardlockup_panic" = 1;
    "kernel.hung_task_all_cpu_backtrace" = 1;
    "kernel.hung_task_panic" = 0;
    "kernel.nmi_watchdog" = 1;
    "kernel.softlockup_all_cpu_backtrace" = 1;
    "kernel.softlockup_panic" = 1;
    "kernel.watchdog" = 1;
  };

  systemd.services.bert-archive = {
    description = "Archive the ACPI Boot Error Record region";
    wantedBy = [ "multi-user.target" ];
    unitConfig = {
      ConditionPathExists = "/sys/firmware/acpi/tables/data/BERT";
      RequiresMountsFor = [ "/var/lib/bert-archive" ];
    };
    serviceConfig = {
      Type = "oneshot";
      StateDirectory = "bert-archive";
    };
    script = ''
      dir=/var/lib/bert-archive
      src=/sys/firmware/acpi/tables/data/BERT
      last=$(ls -1 "$dir" | tail -n 1)
      if [ -n "$last" ] && [ "$(sha256sum < "$src")" = "$(sha256sum < "$dir/$last")" ]; then
        exit 0
      fi
      cp "$src" "$dir/$(date +%Y%m%d-%H%M%S)-$(cat /proc/sys/kernel/random/boot_id).bin"
      ls -1 "$dir" | head -n -64 | while read -r f; do
        rm -- "$dir/$f"
      done
    '';
  };

}
