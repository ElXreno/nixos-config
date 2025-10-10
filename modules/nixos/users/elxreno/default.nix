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
  cfg = config.${namespace}.user.elxreno;
in
{
  options.${namespace}.user.elxreno = {
    enable = mkEnableOption "Whether to enable ElXreno user." // {
      default = true;
    };
    extraGroups = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = "The user's auxiliary groups.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.user.users.elxreno = {
      description = "ElXreno";
      extraGroups = [
        "wheel"
        "networkmanager"
        "dialout"
      ]
      ++ cfg.extraGroups;
      hashedPasswordFile = mkIf (!virtual) config.sops.secrets."user_passwords/elxreno".path;
      openssh.authorizedKeys.keys = [
        "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAH/QtzrqDZ/isIpMslg5FJvT6BoyeqpmiaDjuzcHaIpTexaq/UK4pAdG7IYvs++6JfdfAToWeU7TnOqRj8eubfFXADNwHC3w7gHjx/w8Yq76gcRG+UU/JtUbphzs2EdWWIupaZV+nFiTSbdGlak4fnnqSLIDhRgNa3pBbvSyf2OdD02bA== elxreno@desktop.local"
        "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIORnogu4KTgFE4yxS7dzJxOnuqsBYci9eNgBAnMP68G2 elxreno@gmail.com"
      ];
      uid = 1000;
    };
  };
}
