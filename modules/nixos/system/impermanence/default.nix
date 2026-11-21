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
    concatMapStrings
    filter
    hasPrefix
    attrValues
    concatMap
    genAttrs
    isString
    ;
  cfg = config.${namespace}.system.impermanence;
  privateDirs = filter (dir: any (p: hasPrefix "${dir}/" (p.directory or p)) cfg.directories) [
    "/var/cache/private"
    "/var/lib/private"
  ];
  persistDatasets = filter (m: isString m && hasPrefix "${cfg.defaultPersistentPath}/" m) (
    concatMap (pool: map (ds: ds.mountpoint or null) (attrValues pool.datasets)) (
      attrValues config.disko.devices.zpool
    )
  );
in
{
  options.${namespace}.system.impermanence = {
    enable = mkEnableOption "impermanence";
    defaultPersistentPath = mkOption {
      type = types.str;
      default = "/mnt/root-persist";
    };

    directories = mkOption {
      type = types.listOf (types.either types.str types.attrs);
      default = [ ];
      description = "Directories kept across reboots, in environment.persistence form.";
    };

    files = mkOption {
      type = types.listOf (types.either types.str types.attrs);
      default = [ ];
      description = "Files kept across reboots, in environment.persistence form.";
    };
  };

  config = mkIf cfg.enable {
    clan.core.vars.sops.secretUploadDirectory = "${cfg.defaultPersistentPath}/var/lib/sops-nix";

    fileSystems = genAttrs ([ cfg.defaultPersistentPath ] ++ persistDatasets) (_: {
      neededForBoot = true;
    });

    environment.persistence.${cfg.defaultPersistentPath} = {
      hideMounts = true;
      directories = [
        "/var/log"
        "/var/lib/nixos"
        "/var/lib/systemd/backlight"
        "/var/lib/systemd/pstore"
        "/var/lib/systemd/timers"
      ]
      ++ (optionals config.systemd.coredump.enable [
        "/var/lib/systemd/coredump"
      ])
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

    system.activationScripts = mkIf (privateDirs != [ ]) {
      "private-dirs-permissions" = {
        deps = [ "specialfs" ];
        text = concatMapStrings (dir: ''
          mkdir -p ${cfg.defaultPersistentPath}${dir}
          chmod 0700 ${cfg.defaultPersistentPath}${dir}
        '') privateDirs;
      };

      "createPersistentStorageDirs".deps = [
        "private-dirs-permissions"
        "users"
        "groups"
      ];
    };
  };
}
