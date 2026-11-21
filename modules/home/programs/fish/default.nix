{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) getExe mkIf mkEnableOption;
  cfg = config.${namespace}.programs.fish;
in
{
  options.${namespace}.programs.fish = {
    enable = mkEnableOption "managing fish";
  };

  config = mkIf cfg.enable {
    programs.nh.enable = true;

    programs.fish = {
      enable = true;
      functions = {
        nix = ''
          if test "$argv[1]" = build
            ${getExe pkgs.nix-output-monitor} $argv
          else
            command nix $argv
          end
        '';
      };
      shellAliases = {
        nrs = "nh os switch";
        nrb = "nh os boot";
        nrt = "nh os test";
        ssr = "sudo systemctl restart";
        sss = "sudo systemctl status";
        sst = "sudo systemctl stop";
      };
    };
  };
}
