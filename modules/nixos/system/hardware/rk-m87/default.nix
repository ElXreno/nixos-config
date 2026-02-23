{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.rk-m87;
in
{
  options.${namespace}.system.hardware.rk-m87 = {
    enable = mkEnableOption "Whether or not to manage stuff for rk-m87 keyboard.";
  };

  config = mkIf cfg.enable {
    services.rk-m87-sync.enable = true;
  };
}
