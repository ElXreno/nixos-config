{
  config,
  options,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    attrNames
    filterAttrs
    isOption
    mapAttrs
    mkEnableOption
    mkIf
    ;

  custom = attrNames (filterAttrs (_: type: type == "directory") (builtins.readDir ./.));
  upstream = removeAttrs (filterAttrs (
    _: o: o ? enable && isOption o.enable
  ) options.programs) custom;
  cfg = config.${namespace}.programs;
in
{
  options.${namespace}.programs = mapAttrs (name: _: {
    enable = mkEnableOption name;
  }) upstream;

  config.programs = mapAttrs (name: _: {
    enable = mkIf cfg.${name}.enable true;
  }) upstream;
}
