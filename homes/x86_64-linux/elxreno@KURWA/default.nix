{ namespace, ... }:
{
  ${namespace} = {
    roles = {
      laptop.enable = true;
    };

    desktop-environments.hyprland.enable = true;

    common-packages.enable = true;

    programs = {
      gpg.enable = true;
      ssh.enable = true;
      bottles.enable = true;
      mangohud.enable = true;
    };

    services = {
      gpg-agent.enable = true;
      syncthing = {
        enable = true;
        settings = {
          devices = {
            SM-938B.id = "54NW2KX-ZL5JFVK-TSF6GXD-W6ZAQCN-VZS3DAJ-EKRLKJG-AX6W6AH-VQPBWA2";
          };
          folders = {
            "~/Sync/Books" = {
              id = "mb33z-6aqm9";
              devices = [ "SM-938B" ];
            };
            "~/Sync/Camera" = {
              id = "29llw-v8myj";
              devices = [ "SM-938B" ];
            };
            "~/Sync/Pictures" = {
              id = "s1iqf-q30lv";
              devices = [ "SM-938B" ];
            };
            "~/Sync/Music" = {
              id = "cnd20-w7p5o";
              devices = [ "SM-938B" ];
            };
            "~/Sync/RandomStuff" = {
              id = "ywnqs-d6y2m";
              devices = [ "SM-938B" ];
            };
          };
        };
      };
    };
  };

  home.stateVersion = "25.05";
}
