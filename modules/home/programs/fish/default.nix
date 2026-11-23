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
          if contains -- "$argv[1]" build shell develop
            ${getExe pkgs.rom} $argv[1] -- $argv[2..]
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
