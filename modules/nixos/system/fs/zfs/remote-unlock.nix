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
    mapAttrs
    filterAttrs
    ;
  cfg = config.${namespace}.system.fs.zfs.remoteUnlock;
in
{
  options.${namespace}.system.fs.zfs.remoteUnlock.enable =
    mkEnableOption "unlocking the encrypted pool over initrd SSH";

  config = mkIf cfg.enable {
    ${namespace}.ports.initrd-ssh = 7172;

    clan.core.vars.generators.zfs-passphrase = {
      files.passphrase.neededFor = "partitioning";
      runtimeInputs = [ pkgs.xkcdpass ];
      script = ''
        xkcdpass -d - -n 8 | tr -d '\n' > $out/passphrase
      '';
    };

    boot.initrd = {
      network = {
        enable = true;
        ssh = {
          enable = true;
          port = config.${namespace}.ports.initrd-ssh;
          authorizedKeys = config.users.users.${config.${namespace}.primaryUser}.openssh.authorizedKeys.keys;
          hostKeys = [
            config.clan.core.vars.generators.openssh.files."ssh.id_ed25519".path
          ];
        };
      };

      systemd.network = {
        enable = true;
        networks = mapAttrs (_: n: {
          inherit (n) matchConfig address routes;
        }) (filterAttrs (_: n: n.address != [ ]) config.systemd.network.networks);
      };
    };
  };
}
