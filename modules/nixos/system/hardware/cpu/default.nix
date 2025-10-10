{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkMerge
    mkOption
    types
    ;
  cfg = config.${namespace}.system.hardware.cpu;
in
{
  options.${namespace}.system.hardware.cpu = {
    enable = mkEnableOption "Whether or not to manage cpu stuff." // {
      default = true;
    };
    manufacturer = mkOption {
      type = types.nullOr (
        types.enum [
          "amd"
          "intel"
        ]
      );
      default = null; # TODO: Utilize facter if possible
      description = "CPU Manufacturer.";
    };
  };

  config = mkIf cfg.enable {
    hardware = {
      cpu = mkMerge [
        (mkIf (cfg.manufacturer == "amd") {
          amd.updateMicrocode = true;
        })
        (mkIf (cfg.manufacturer == "intel") {
          intel.updateMicrocode = true;
        })
      ];
    };
  };
}
