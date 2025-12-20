{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    mkEnableOption
    mkOption
    types
    mkIf
    optionals
    any
    hasPrefix
    ;
  cfg = config.${namespace}.system.impermanence;
in
{
  options.${namespace}.system.impermanence = {
    enable = mkEnableOption "Whether to enable impermanence.";
    defaultPersistentPath = mkOption {
      type = types.str;
      default = "/mnt/root-persist";
    };

    directories = mkOption {
      default = [ ];
    };

    files = mkOption {
      default = [ ];
    };
  };

  config = mkIf cfg.enable {
    environment.persistence.${cfg.defaultPersistentPath} = {
      hideMounts = true;
      directories = [
        "/etc/ssh"
        "/var/log"
        "/var/lib/nixos"
      ]
      # TODO: Migrate
      ++ (optionals config.services.power-profiles-daemon.enable [
        "/var/lib/power-profiles-daemon"
      ])
      ++ (optionals config.services.colord.enable [
        {
          directory = "/var/lib/colord";
          user = "colord";
          group = "colord";
          mode = "u=rwx,g=rx,o=";
        }
      ])
      ++ cfg.directories;
      files = [
        "/etc/machine-id"
        "/etc/ssh/ssh_host_ed25519_key"
        "/etc/ssh/ssh_host_ed25519_key.pub"
        "/etc/ssh/ssh_host_rsa_key"
        "/etc/ssh/ssh_host_rsa_key.pub"
      ]
      ++ cfg.files;
    };

    system.activationScripts =
      mkIf (any (p: hasPrefix "/var/lib/private" (p.directory or p)) cfg.directories)
        {
          "var-lib-private-permissions" = {
            deps = [ "specialfs" ];
            text = ''
              mkdir -p ${cfg.defaultPersistentPath}/var/lib/private
              chmod 0700 ${cfg.defaultPersistentPath}/var/lib/private
            '';
          };

          "createPersistentStorageDirs".deps = [
            "var-lib-private-permissions"
            "users"
            "groups"
          ];
        };
  };
}
