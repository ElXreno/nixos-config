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
    literalExpression
    ;
  cfg = config.${namespace}.system.boot.kernel;
in
{
  options.${namespace}.system.boot.kernel = {
    enable = mkEnableOption "Whether or not to manage kernel." // {
      default = true;
    };
    packages = mkOption {
      type = types.raw;
      default = pkgs.linuxPackages_latest;
      defaultText = literalExpression "pkgs.linuxPackages_latest";
    };
  };

  config = mkIf cfg.enable {
    boot.kernelPackages = cfg.packages;
  };
}
