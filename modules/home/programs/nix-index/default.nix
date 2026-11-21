{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.nix-index;
in
{
  options.${namespace}.programs.nix-index = {
    enable = mkEnableOption "managing nix-index";
  };

  config = mkIf cfg.enable {
    programs = {
      nix-index = {
        enable = true;
      };
      nix-index-database.comma.enable = true;
    };
  };
}
