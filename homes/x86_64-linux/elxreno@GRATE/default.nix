{ namespace, ... }:
{
  ${namespace} = {
    roles.server.enable = true;

    programs.htop.showAdvancedCPUStats = true;

    services = {
      gpg-agent.enable = true;
      syncthing = {
        enable = true;
        settings = {
          devices = {
            KURWA.id = "NKP6O5C-IURZ7YR-2IGNDRG-SMP7OXJ-3GJAEUV-EJVART4-MBQ6EMG-PX3G6QE";
            SM-938B.id = "54NW2KX-ZL5JFVK-TSF6GXD-W6ZAQCN-VZS3DAJ-EKRLKJG-AX6W6AH-VQPBWA2";
          };
          folders = {
            "~/Sync/Books" = {
              id = "mb33z-6aqm9";
              devices = [
                "KURWA"
                "SM-938B"
              ];
              type = "receiveonly";
            };
            "~/Sync/Camera" = {
              id = "29llw-v8myj";
              devices = [
                "KURWA"
                "SM-938B"
              ];
              type = "receiveonly";
            };
            "~/Sync/Pictures" = {
              id = "s1iqf-q30lv";
              devices = [
                "KURWA"
                "SM-938B"
              ];
              type = "receiveonly";
            };
            "~/Sync/Music" = {
              id = "cnd20-w7p5o";
              devices = [
                "KURWA"
                "SM-938B"
              ];
              type = "receiveonly";
            };
            "~/Sync/RandomStuff" = {
              id = "ywnqs-d6y2m";
              devices = [
                "KURWA"
                "SM-938B"
              ];
              type = "receiveonly";
            };
            "~/projects" = {
              id = "ygtua-856sad";
              devices = [ "KURWA" ];
              type = "receiveonly";
            };
          };
        };
      };
    };
  };

  home.stateVersion = "25.05";
}
