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
    mkOption
    types
    concatStringsSep
    escapeShellArg
    ;
  cfg = config.${namespace}.services.modprobed-db;

  generatedCfg = ''
    DBPATH=${escapeShellArg cfg.settings.stateDir}
    ${lib.optionalString (cfg.settings.colors != null) "COLORS=${escapeShellArg cfg.settings.colors}"}
    IGNORE=(${(concatStringsSep " " (map escapeShellArg cfg.settings.ignore))})
  '';
in
{
  options.${namespace}.services.modprobed-db = {
    enable = mkEnableOption "Whether or not to manage modprobed-db.";
    package = mkOption {
      type = types.package;
      default = pkgs.modprobed-db;
      description = "modprobed-db package to use.";
    };

    dates = mkOption {
      type = types.str;
      default = "*-*-* *:*:00";
      example = "hourly";
      description = ''
        How often run modprobed-db.

        The format is described in {manpage}`systemd.time(7)`.
      '';
    };

    # https://github.com/graysky2/modprobed-db/blob/master/common/modprobed-db.skel
    settings = {
      colors = mkOption {
        type = types.enum [
          "dark"
          "light"
        ];
        default = "dark";
        description = "Output theme for modprobed-db.";
      };
      ignore = mkOption {
        type = types.listOf types.str;
        default = [
          "nvidia"
          "nvidia_drm"
          "nvidia_modeset"
          "nvidia_uvm"
          "vboxdrv"
          "vboxnetadp"
          "vboxnetflt"
          "vboxpci"
        ];
        description = "Kernel modules to exclude from the database.";
      };
      stateDir = mkOption {
        type = types.path;
        default = "/var/lib/modprobed-db";
        description = "Database state directory.";
      };
    };
    user = mkOption {
      description = "User running modprobed-db";
      type = types.str;
      default = "modprobed-db";
    };
    group = mkOption {
      description = "Group of user running modprobed-db";
      type = types.str;
      default = "modprobed-db";
    };
  };

  config = mkIf cfg.enable {
    environment.etc."xdg/modprobed-db/modprobed-db.conf" = {
      text = generatedCfg;
      mode = "0440";
      user = cfg.user;
      group = cfg.group;
    };

    systemd = {
      services.modprobed-db = {
        description = "modprobed-db service to scan and store new kernel modules.";
        wantedBy = [ "multi-user.target" ];
        wants = [ "modprobed-db.timer" ];
        path = with pkgs; [
          getent
          gawk
        ];
        environment.XDG_CONFIG_HOME = "/etc/xdg/modprobed-db/";

        startAt = cfg.dates;

        serviceConfig = {
          Type = "simple";
          DynamicUser = true;
          User = cfg.user;
          Group = cfg.group;
          Restart = "on-failure";

          WorkingDirectory = cfg.settings.stateDir;
          StateDirectory = "modprobed-db";

          ExecStart = "${pkgs.modprobed-db}/bin/modprobed-db store";
        };
      };

      timers.modprobed-db = {
        timerConfig = {
          Persistent = true;
        };
      };
    };

    users = {
      users = mkIf (cfg.user == "modprobed-db") {
        modprobed-db = {
          group = cfg.group;
          isSystemUser = true;
        };
      };
      groups = mkIf (cfg.group == "modprobed-db") { modprobed-db = { }; };
    };
  };
}
