{ config, pkgs, ... }:
let
  defaultMountOptions = [ "compress-force=zstd" ];
in
{
  clan.core.vars.generators.luks = {
    files.key.neededFor = "partitioning";
    runtimeInputs = [ pkgs.xkcdpass ];
    script = ''
      xkcdpass -d - -n 8 | tr -d '\n' > $out/key
    '';
  };

  boot.initrd = {
    systemd = {
      enable = true;
      network = {
        enable = true;
        networks."10-ens3" = {
          matchConfig.Name = "ens3";
          address = [
            "159.195.56.52/22"
            "2a0a:4cc0:c1:e9be::/64"
          ];
          routes = [
            { Gateway = "159.195.56.1"; }
            { Gateway = "fe80::1"; }
          ];
        };
      };
    };

    network = {
      enable = true;
      ssh = {
        enable = true;
        port = 7172;
        authorizedKeys = config.users.users.elxreno.openssh.authorizedKeys.keys;
        hostKeys = [
          config.clan.core.vars.generators.openssh.files."ssh.id_ed25519".path
        ];
      };
    };

    kernelModules = [ "virtio_net" ];
  };

  disko.devices = {
    disk = {
      vda = {
        type = "disk";
        device = "/dev/vda";
        content = {
          type = "gpt";
          partitions = {
            ESP = {
              size = "1024M";
              type = "EF00";
              priority = 1;
              content = {
                type = "filesystem";
                format = "vfat";
                mountpoint = "/boot";
                mountOptions = [ "umask=0077" ];
              };
            };
            root = {
              size = "100%";
              content = {
                type = "luks";
                name = "cryptroot";
                passwordFile = config.clan.core.vars.generators.luks.files.key.path;
                settings.allowDiscards = true;
                content = {
                  type = "btrfs";
                  extraArgs = [ "-f" ];
                  subvolumes = {
                    "/root" = {
                      mountpoint = "/";
                      mountOptions = defaultMountOptions;
                    };
                    "/nix" = {
                      mountpoint = "/nix";
                      mountOptions = defaultMountOptions;
                    };
                    "/home" = {
                      mountpoint = "/home";
                      mountOptions = defaultMountOptions;
                    };
                  };
                };
              };
            };
          };
        };
      };
    };
  };
}
