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
    replaceStrings
    getExe
    ;
  cfg = config.${namespace}.programs.anyrun;

  preprocessScript = pkgs.writeShellScriptBin "anyrun-preprocess-application-exec" ''
    shift
    echo "${lib.getExe pkgs.uwsm} app -- $*"
  '';
in
{
  options.${namespace}.programs.anyrun = {
    enable = mkEnableOption "Whether or not to manage anyrun.";
    package = mkPackageOption pkgs "anyrun" { };

    daemon.enable =
      mkEnableOption "Enable running Anyrun as a daemon, allowing for faster startup speed."
      // {
        default = true;
      };
  };

  config = mkIf cfg.enable {
    programs.anyrun = {
      enable = true;
      package = cfg.package;

      config = {
        plugins =
          map
            (
              name:
              let
                plugin = replaceStrings [ "-" ] [ "_" ] name;
              in
              "${cfg.package}/lib/lib${plugin}.so"
            )
            [
              "applications"
              "dictionary"
              "nix-run"
              "rink"
              "translate"
              "niri-focus"
            ];
      };

      extraConfigFiles = {
        "applications.ron".text = ''
          Config(
            desktop_actions: false,
            max_entries: 5,
            hide_description: false,
            terminal: Some(Terminal(
              command: "${lib.getExe pkgs.xdg-terminal-exec}",
              args: "{}"
            )),
            preprocess_exec_script: Some("${lib.getExe preprocessScript}"),
          )
        '';
      };

      extraCss =
        let
          col = sel: config.lib.stylix.colors.withHashtag.${sel};
        in
        ''
          @define-color accent ${col "base0D"};
          @define-color bg-color ${col "base00"};
          @define-color fg-color ${col "base05"};
          @define-color desc-color ${col "base04"};

          window {
            background: transparent;
            animation: slide-in 0.2s ease-out;
          }

          box.main {
            padding: 5px;
            margin: 10px;
            border-radius: 10px;
            border: 2px solid @accent;
            background-color: @bg-color;
            box-shadow: 0 0 5px black;
          }

          text {
            min-height: 30px;
            padding: 5px;
            border-radius: 5px;
            color: @fg-color;
            transition: all 0.2s ease;
          }

          .matches {
            background-color: transparent;
            border-radius: 10px;
            transition: all 0.1s ease;
          }

          box.plugin:first-child {
            margin-top: 5px;
          }

          box.plugin.info {
            min-width: 200px;
          }

          list.plugin {
            background-color: transparent;
          }

          label.match {
            color: @fg-color;
            transition: color 0.2s;
          }

          label.match.description {
            font-size: 10px;
            color: @desc-color;
            transition: color 0.2s;
          }

          label.plugin.info {
            font-size: 14px;
            color: @fg-color;
          }

          .match {
            background: transparent;
            border-left: 0px solid transparent;
            transition: all 0.1s ease-out;
            padding: 3px;
          }

          /* Like .match:hover but bugless */
          row:hover {
            background: alpha(@accent, 0.1);
            border-radius: 5px;
          }

          .match:selected {
            border-left: 4px solid @accent;
            background: alpha(@accent, 0.2);
            border-radius: 0 5px 5px 0;
            transition: all 0.1s ease;
          }

          @keyframes slide-in {
            0% {
              opacity: 0;
              transform: translateY(-20px);
            }
            100% {
              opacity: 1;
              transform: translateY(0);
            }
          }
        '';
    };

    systemd.user.services.anyrun = mkIf cfg.daemon.enable {
      Unit = {
        Description = "Anyrun daemon";
        PartOf = "graphical-session.target";
        After = "graphical-session.target";
      };

      Service = {
        Type = "simple";
        ExecStart = "${getExe cfg.package} daemon";
        Restart = "on-failure";
        KillMode = "process";
      };

      Install = {
        WantedBy = [ "graphical-session.target" ];
      };
    };
  };
}
