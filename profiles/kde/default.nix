{ config, inputs, pkgs, lib, ... }:
{
  imports = with inputs.self.nixosProfiles; [ xserver kdeconnect ];
  services.xserver = {
    displayManager = {
      sddm.enable = true;
      # https://github.com/NixOS/nixpkgs/pull/143673
      # defaultSession = "plasma";
    };
    desktopManager.plasma5 = {
      enable = true;
    };
  };

  services.colord.enable = config.device == "INFINITY";

  environment.systemPackages = with pkgs; [
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

    # Fonts
    jetbrains-mono

    ## For telegram font fix
    open-sans
  ] ++ lib.optional config.services.colord.enable gnome.gnome-color-manager;

  home-manager.users.elxreno = {
    imports = [ inputs.plasma-manager.homeManagerModules.plasma-manager ];
    programs.plasma = {
      enable = true;
      files = {
        # Yeah, it works without [$e] which got converted to \x5b$e\x5d by kwriteconfig5 oof
        baloofilerc.General."exclude folders" = "$HOME/android-build/,$HOME/projects/";

        dolphinrc = {
          General = {
            GlobalViewProps = false;
            ShowFullPath = true;
          };
        };

        kdeglobals.Icons.Theme = "Papirus";

        kscreenlockerrc = {
          Daemon = {
            Timeout = 10; # min
            LockGrace = 7; # sec
          };
        };

        ksmserverrc.General.loginMode = "restoreSavedSession";

        konsolerc = {
          TabBar = {
            NewTabBehavior = "PutNewTabAfterCurrentTab";
            TabBarVisibility = "AlwaysShowTabBar";
          };
        };

        kwinrc.NightColor.Active = true;

        powermanagementprofilesrc = {
          "AC.DPMSControl".idleTime = 780; # 13 min
          "AC.DimDisplay".idleTime = 480000; # 8 min
          "AC.HandleButtonEvents" = {
            lidAction = 32;
            triggerLidActionWhenExternalMonitorPresent = false;
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
          "Containments.2.Applets.19.Configuration.Appearance".use24hFormat = 2;
        };
      };
    };
  };
}
