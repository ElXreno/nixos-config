{
  config,
  lib,
  pkgs,
  ...
}:
{
  sops.secrets =
    let
      mkPasswords = lib.mkMerge (
        lib.lists.forEach
          (builtins.filter (
            user:
            let
              inherit (config.users.users."${user}") uid;
            in
            uid != null && uid >= 1000 && uid <= 1010
          ) (builtins.attrNames config.users.users))
          (user: {
            "user_passwords/${user}" = {
              neededForUsers = true;
              key = "user_passwords/${user}";
            };
          })
      );
    in
    mkPasswords;

  users = {
    mutableUsers = false;
    users = {
      elxreno = {
        description = "ElXreno";
        extraGroups = [
          "wheel"
          "networkmanager"
          "libvirtd"
          "adbusers"
          "docker"
          "dialout"
          "cdrom"
        ];
        hashedPasswordFile = lib.mkDefault config.sops.secrets."user_passwords/elxreno".path;
        openssh.authorizedKeys.keys = [
          "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAH/QtzrqDZ/isIpMslg5FJvT6BoyeqpmiaDjuzcHaIpTexaq/UK4pAdG7IYvs++6JfdfAToWeU7TnOqRj8eubfFXADNwHC3w7gHjx/w8Yq76gcRG+UU/JtUbphzs2EdWWIupaZV+nFiTSbdGlak4fnnqSLIDhRgNa3pBbvSyf2OdD02bA== elxreno@desktop.local"
          "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIORnogu4KTgFE4yxS7dzJxOnuqsBYci9eNgBAnMP68G2 elxreno@gmail.com"
        ];
        shell = pkgs.fish;
        isNormalUser = true;
        uid = 1000;
      };

      alena = lib.mkIf (config.device == "AMD-Desktop" || config.device == "INFINITY") {
        description = "Alena";
        extraGroups = [
          "wheel"
          "networkmanager"
        ];
        hashedPasswordFile = lib.mkDefault config.sops.secrets."user_passwords/alena".path;
        openssh.authorizedKeys.keys = config.users.users.elxreno.openssh.authorizedKeys.keys;
        isNormalUser = true;
        uid = 1001;
      };
    };
  };
}
