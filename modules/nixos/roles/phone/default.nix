{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.phone;
in
{
  options.${namespace}.roles.phone = {
    enable = mkEnableOption "phone configuration";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      system = {
        boot.enable = false;
        zswap.enable = false;
      };
    };

    users.users.root.linger = true;
  };
}
