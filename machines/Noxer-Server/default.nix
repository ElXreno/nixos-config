{ config, pkgs, inputs, lib, ... }:
{
  imports = [
    inputs.self.nixosRoles.server
    # inputs.self.nixosProfiles.boinc
    inputs.self.nixosProfiles.nginx
    # inputs.self.nixosProfiles.ipfs
    ./hardware-configuration.nix
    ./minecraft-server.nix
    ./smart-home.nix
    ./wireguard.nix
  ];

  # Oracle Cloud uses EFI boot
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;

  services.beesd.filesystems = {
    root = {
      spec = "LABEL=nixos";
      verbosity = "crit";
      workDir = "beeshome";
    };
  };

  services.gitea = {
    enable = true;
    appName = config.services.gitea.domain;
    cookieSecure = true;
    disableRegistration = true;
    domain = "code.elxreno.ninja";
    lfs.enable = true;
    rootUrl = "https://${config.services.gitea.domain}";
    settings = {
      indexer = {
        REPO_INDEXER_ENABLED = true;
      };
      "cron.repo_health_check" = {
        TIMEOUT = "180m";
      };
      "cron.git_gc_repos" = {
        ENABLED = true;
        SCHEDULE = "@every 168h";
        TIMEOUT = "180m";
        ARGS = "--auto --aggressive";
      };
    };
  };

  services.postgresql = {
    enable = true;
    package = pkgs.postgresql_11;
  };

  sops.secrets.coturn = { };

  services.matrix-synapse = {
    enable = true;
    settings = {
      server_name = "elxreno.ninja";
      public_baseurl = "https://matrix.elxreno.ninja";
      listeners = [
        {
          bind_addresses = [ "127.0.0.1" ];
          port = 8008;
          resources = [
            {
              names = [ "client" "federation" ];
              compress = false;
            }
          ];
          tls = false;
          type = "http";
          x_forwarded = true;
        }
      ];
      turn_uris = [
        "turn:turn.elxreno.ninja:5349?transport=udp"
        "turn:turn.elxreno.ninja:5350?transport=udp"
        "turn:turn.elxreno.ninja:5349?transport=tcp"
        "turn:turn.elxreno.ninja:5350?transport=tcp"
      ];
      turn_user_lifetime = "1h";
      url_preview_enabled = true;
      max_upload_size = "100M";
    };
    extraConfigFiles = [ config.sops.secrets.coturn.path ];
  };

  # networking.firewall.allowedTCPPorts = [ 8448 ];

  services.nginx = {
    clientMaxBodySize = config.services.matrix-synapse.settings.max_upload_size;
    virtualHosts = {
      "elxreno.ninja" = {
        locations."= /.well-known/matrix/server".extraConfig =
          let
            server = { "m.server" = "matrix.elxreno.ninja:443"; };
          in
          ''
            add_header Content-Type application/json;
            return 200 '${builtins.toJSON server}';
          '';
        locations."= /.well-known/matrix/client".extraConfig =
          let
            client = {
              "m.homeserver" = { "base_url" = "https://matrix.elxreno.ninja"; };
              "m.identity_server" = { "base_url" = "https://vector.im"; };
            };
          in
          ''
            add_header Content-Type application/json;
            add_header Access-Control-Allow-Origin *;
            return 200 '${builtins.toJSON client}';
          '';
      };
      "matrix.elxreno.ninja" = {
        addSSL = true;
        enableACME = true;

        locations."/" = {
          proxyPass = "http://127.0.0.1:8008";
        };
      };
      "element.elxreno.ninja" = {
        addSSL = true;
        enableACME = true;
        root = pkgs.element-web.override {
          conf = {
            default_server_config."m.homeserver" = {
              "base_url" = "https://matrix.elxreno.ninja";
              "server_name" = "elxreno.ninja";
            };
          };
        };
      };
    };
  };

  users.groups.nginx.members = [ "matrix-synapse" ];

  security.acme.certs = {
    "matrix.elxreno.ninja" = {
      postRun = "systemctl reload nginx.service; systemctl restart matrix-synapse.service";
    };
  };

  services.postgresqlBackup = {
    enable = true;
    backupAll = true;
    compression = "zstd";
  };

  sops.secrets."restic/gdrive_password" = { };

  services.restic.backups = {
    noxer-backups = {
      repository = "rclone:gdrive:noxer-backups";
      rcloneConfigFile = "/home/elxreno/.config/rclone/rclone.conf";
      passwordFile = config.sops.secrets."restic/gdrive_password".path;
      paths = [
        "/var/backup/postgresql"
        "/var/lib/acme"
        "/var/lib/gitea"
        "/var/lib/matrix-synapse"
      ];
      pruneOpts = [
        "--keep-daily 7"
      ];
      timerConfig = {
        OnCalendar = "01:30";
        Persistent = true;
      };
      extraBackupArgs = [
        "--exclude=/var/lib/gitea/data/indexers"
        "--exclude=/var/lib/gitea/repositories/elxreno/android_frameworks_base.git"
        "--exclude=/var/lib/gitea/repositories/elxreno/android_vendor_aospa.git"
        "--exclude=/var/lib/gitea/repositories/elxreno/nixpkgs.git"
      ];
    };
  };

  # college dev
  # services.postgresql = {
  #   authentication = pkgs.lib.mkOverride 10 ''
  #     local all all trust
  #     host all all ::1/128 trust
  #   '';
  # };
  # 
  # services.smart-home-server = {
  #   enable = true;
  #   addToNginx = true;
  #   nginxVirtualHost = "api.elxreno.ninja";
  # };

  system.stateVersion = "22.05";
}
