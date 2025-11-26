{
  config,
  namespace,
  lib,
  virtual,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.sops;
in
{
  options.${namespace}.sops = {
    enable = mkEnableOption "Whether to enable sops.";
    provisionUserPasswords = mkEnableOption "Whether to provision user hashed passwords as file." // {
      default = !virtual;
    };
  };

  config = mkIf cfg.enable {
    sops = {
      defaultSopsFile = mkIf (!virtual) (
        lib.snowfall.fs.get-file (
          if config.${namespace}.roles.server.enable then "secrets/server.yaml" else "secrets/common.yaml"
        )
      );
      age.sshKeyPaths = [ "/etc/ssh/ssh_host_ed25519_key" ];
      age.keyFile = "/var/lib/sops-nix/key";
      age.generateKey = true;

      secrets =
        let
          mkPasswords = lib.mkMerge (
            lib.lists.forEach
              (builtins.filter (
                user:
                let
                  inherit (config.${namespace}.user.users."${user}") uid;
                in
                uid != null && uid >= 1000 && uid <= 1010
              ) (builtins.attrNames config.${namespace}.user.users))
              (user: {
                "user_passwords/${user}" = {
                  neededForUsers = true;
                  key = "user_passwords/${user}";
                };
              })
          );
        in
        mkIf cfg.provisionUserPasswords mkPasswords;
    };
  };
}
