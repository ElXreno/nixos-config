{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.git;
in
{
  options.${namespace}.programs.git = {
    enable = mkEnableOption "managing git";
  };

  config = mkIf cfg.enable {
    home.packages = with pkgs; [ b4 ];

    programs.git = {
      enable = true;
      lfs.enable = true;
      package = pkgs.gitFull;

      settings = {
        alias = {
          cp = "cherry-pick";
        };

        pull.rebase = true;
        init.defaultBranch = "main";
        fetch.prune = true;
        core.autocrlf = "input";
        gc.auto = 0;
      };
    };
  };
}
