{
  namespace,
  ...
}:

{
  imports = [
    ./network.nix
    ./llm.nix
  ];

  ${namespace} = {
    tailscale.trusted = false;

    system = {
      ccache = {
        enable = true;
        cacheDir = "/var/cache/ccache";
        aliases = [ "/nix/ccache" ];
      };
      networking.dns.addresses = {
        v4 = "159.195.56.52";
        v6 = "2a0a:4cc0:c1:e9be::";
      };

      networking.tunnels = {
        warp.enable = true;
        airvpn = {
          mtu = 1440;
          # Piautos and Amansinaya, entry 3
          endpoints = [
            "134.19.178.169:1637"
            "146.70.179.196:1637"
          ];
        };
      };

      fs.zfs = {
        hostId = "0fa43409";
        remoteUnlock.enable = true;
        autoSnapshot.filesystems = {
          "rpool/home-persist" = true;
          "rpool/root-persist" = true;
          "rpool/root-persist/matrix" = true;
          "rpool/root-persist/minecraft" = true;
          "rpool/root-persist/postgres" = true;
        };
      };
    };

    services = {
      clickhouse = {
        nativePort = 9010;
        httpPort = 8124;
        memoryRatio = 0.9;
        maxMemory = 9663676416;
      };
      matrix.enable = false;
      niks3 = {
        enable = true;
        capacity = "500GB";
      };
      nixbot = {
        enable = true;
        repositories = {
          "ElXreno/nixos-config".url = "https://github.com/ElXreno/nixos-config";
          "ElXreno/nixos-config-auto-update" = {
            url = "https://github.com/ElXreno/nixos-config";
            branch = "auto-update";
          };
        };
      };
      minecraft.atm10.enable = false;
      minecraft.atm10-sky.enable = false;
      minecraft.gtnh.enable = false;
      caddy = {
        enable = true;
        enableDefaultVhost = true;
      };

      ripe-atlas.enable = true;

      roj = {
        enable = true;
        forwardedPort = 38320;
        scalingFactor = 25;
        web = {
          enable = true;
          serve = true;
          sharePort = 8443;
          posters = true;
          owners = [ "ElXreno@github" ];
          classifier = true;
          engine = true;
          reindexMemory = 4000000000;
        };
      };
    };
  };
}
