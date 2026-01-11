{ namespace, ... }:
{
  ${namespace} = {
    roles = {
      laptop.enable = true;
    };

    desktop-environments.niri.enable = true;

    common-packages.enable = true;

    programs = {
      gpg.enable = true;
      ssh.enable = true;
      bottles.enable = true;
      mangohud.enable = true;

      # Integrated graphics is more power efficient
      # See `mpv --vulkan-device=help`
      mpv.vulkanDevice = "AMD Radeon 780M Graphics (RADV PHOENIX)";
    };

    services = {
      gpg-agent.enable = true;
      syncthing = {
        enable = true;
        settings = {
          devices = {
            GRATE.id = "WAPSKKS-INQGXIL-76BTAK7-XMJPFZK-EKNP4H4-F3TYR7R-PYSESF4-6NEGVAL";
            INFINITY.id = "5HO6RO2-TWEE7LQ-CHOYWXN-TDNIMRK-BU6Z2IB-BZZ5LNM-TKDAI36-HFXUMAL";
            SM-938B.id = "54NW2KX-ZL5JFVK-TSF6GXD-W6ZAQCN-VZS3DAJ-EKRLKJG-AX6W6AH-VQPBWA2";
          };
          folders = {
            "~/Sync/Books" = {
              id = "mb33z-6aqm9";
              devices = [
                "GRATE"
                "INFINITY"
                "SM-938B"
              ];
            };
            "~/Sync/Camera" = {
              id = "29llw-v8myj";
              devices = [
                "GRATE"
                "INFINITY"
                "SM-938B"
              ];
            };
            "~/Sync/Pictures" = {
              id = "s1iqf-q30lv";
              devices = [
                "GRATE"
                "INFINITY"
                "SM-938B"
              ];
            };
            "~/Sync/Music" = {
              id = "cnd20-w7p5o";
              devices = [
                "GRATE"
                "INFINITY"
                "SM-938B"
              ];
            };
            "~/Sync/RandomStuff" = {
              id = "ywnqs-d6y2m";
              devices = [
                "GRATE"
                "INFINITY"
                "SM-938B"
              ];
            };
          };
        };
      };
    };
  };

  home.stateVersion = "25.05";
}
