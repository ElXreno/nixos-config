{ config, inputs, pkgs, lib, ... }:
{
  imports = with inputs.self.nixosProfiles; [ xserver kdeconnect ];
  services.xserver = {
    displayManager = {
      sddm.enable = true;
      defaultSession = lib.mkIf (config.device == "INFINITY") "plasmawayland";
    };
    desktopManager.plasma5 = {
      enable = true;
    };
  };

  environment.plasma5.excludePackages = with pkgs; [ elisa ];

  services.colord.enable = config.device == "INFINITY";

  fonts.fonts = with pkgs; [
    jetbrains-mono
  ];

  environment.systemPackages = with pkgs; [
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

    # Ark dependency
    unrar

    # Icons
    papirus-icon-theme
  ] ++ lib.optional config.services.colord.enable gnome.gnome-color-manager;

  programs.dconf.enable = true;

  home-manager.users.elxreno = {
    imports = [ inputs.plasma-manager.homeManagerModules.plasma-manager ];
    programs.plasma = {
      enable = true;
      files = {
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

        kcminputrc."Libinput.10182.480.GXTP7863:00 27C6:01E0 Touchpad" = lib.mkIf (config.device == "INFINITY") {
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

        powermanagementprofilesrc = {
          "AC.DPMSControl".idleTime = 780; # 13 min
          "AC.DimDisplay".idleTime = 480000; # 8 min
          "AC.HandleButtonEvents" = {
            lidAction = 32;
            triggerLidActionWhenExternalMonitorPresent = false;
          };
          "AC.SuspendSession" = {
            idleTime = null;
            suspendThenHibernate = null;
            suspendType = null;
          };
          "Battery.DPMSControl".idleTime = 600; # 10 min
          "Battery.DimDisplay".idleTime = 300000; # 5 min
          "Battery.HandleButtonEvents".triggerLidActionWhenExternalMonitorPresent = false;
          "Battery.SuspendSession" = {
            idleTime = null;
            suspendThenHibernate = null;
            suspendType = null;
          };
          "LowBattery.SuspendSession".suspendThenHibernate = false;
        };

        # Applets
        "plasma-org.kde.plasma.desktop-appletsrc" = {
          "Containments.2.Applets.18.Configuration.Appearance".use24hFormat = 2; # TODO: Maybe I should change regional settings instead?
        };
      };
    };
  };
}
