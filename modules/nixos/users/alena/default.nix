{
  config,
  namespace,
  lib,
  virtual,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.user.alena;
in
{
  options.${namespace}.user.alena = {
    enable = mkEnableOption "Whether to enable Alena user.";
    extraGroups = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = "The user's auxiliary groups.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.user.users.alena = {
      description = "Alena";
      extraGroups = [
        "wheel"
        "networkmanager"
      ]
      ++ cfg.extraGroups;
      hashedPasswordFile = mkIf (!virtual) config.sops.secrets."user_passwords/alena".path;
      openssh.authorizedKeys.keys = config.users.users.elxreno.openssh.authorizedKeys.keys;
      uid = 1001;
    };
  };
}
