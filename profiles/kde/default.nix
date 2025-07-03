{
  config,
  inputs,
  pkgs,
  lib,
  ...
}:
{
  imports = with inputs.self.nixosProfiles; [
    services.xserver
    home.services.kdeconnect
  ];
  services = {
    displayManager = {
      sddm = {
        enable = true;
        wayland.enable = config.device != "AMD-Desktop";
      };

      defaultSession = lib.mkIf (config.device == "AMD-Desktop") "plasmax11";
    };
    desktopManager.plasma6 = {
      enable = true;
    };
  };

  environment.plasma6.excludePackages = with pkgs; [ kdePackages.elisa ];

  services.colord.enable = config.device == "INFINITY";

  fonts.packages = with pkgs; [ jetbrains-mono ];

  environment.systemPackages =
    with pkgs;
    with kdePackages;
    [
      # Utilities for Info Center
      clinfo
      glxinfo
      vulkan-tools

      ark
      gwenview
      kate
      kompare
      kleopatra
      okular
      okteta
      kdiskmark

      # Ark dependency
      unrar

      # Icons
      papirus-icon-theme
    ]
    ++ lib.optional config.services.colord.enable gnome-color-manager;

  programs.dconf.enable = true;

  home-manager.users.elxreno = {
    home.sessionVariables = lib.mkIf (config.device == "INFINITY") {
      NIXOS_OZONE_WL = 1;
    };
    imports = [ inputs.plasma-manager.homeManagerModules.plasma-manager ];
    programs.plasma = {
      enable = true;
      configFile = {
        baloofilerc = {
          "Basic Settings"."Indexing-Enabled" = false;
          # Yeah, it works without [$e] which got converted to \x5b$e\x5d by kwriteconfig5 oof
          # upd at 06/06/23: Fixed by upstream
          General."exclude folders" = "$HOME/android-build/,$HOME/projects/";
        };

        dolphinrc = {
          General = {
            GlobalViewProps = false;
            ShowFullPath = true;
          };
        };

        kdeglobals = {
          Icons.Theme = "Papirus";
          KDE.SingleClick = true;
        };

        kscreenlockerrc = {
          Daemon = {
            Timeout = 10; # min
            LockGrace = 7; # sec
          };
        };

        kcminputrc."Libinput.10182.480.GXTP7863:00 27C6:01E0 Touchpad" =
          lib.mkIf (config.device == "INFINITY")
            {
              NaturalScroll = true;
              ScrollFactor = 0.5;
              TapToClick = true;
            };

        ksmserverrc.General.loginMode = "restoreSavedSession";

        konsolerc = {
          "Desktop Entry".DefaultProfile = "default.profile";

          KonsoleWindow.RememberWindowSize = false;

          TabBar = {
            NewTabBehavior = "PutNewTabAfterCurrentTab";
            TabBarVisibility = "AlwaysShowTabBar";
            TabBarPosition = "Bottom";
          };
        };
        "../.local/share/konsole/default.profile" = {
          Appearance.Font = "JetBrains Mono,10,-1,5,50,0,0,0,0,0";
          General = {
            Name = "default";
            TerminalColumns = 120;
            TerminalRows = 30;
          };
          Scrolling = {
            HistorySize = 50000;
          };
        };

        kwinrc = {
          NightColor.Active = true;
          Xwayland.Scale = 1;
        };
      };
    };
  };
}
