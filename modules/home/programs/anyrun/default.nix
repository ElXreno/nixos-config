{
  config,
  namespace,
  lib,
  inputs,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.anyrun;

  anyrunPkgs = inputs.anyrun.packages.${pkgs.stdenv.hostPlatform.system};
in
{
  options.${namespace}.programs.anyrun = {
    enable = mkEnableOption "Whether or not to manage anyrun.";
  };

  config = mkIf cfg.enable {
    programs.anyrun = {
      enable = true;

      config = {
        plugins = with anyrunPkgs; [
          applications
          dictionary
          nix-run
          rink
          translate
          niri-focus
        ];
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
  };
}
