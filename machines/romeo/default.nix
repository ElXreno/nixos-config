{ config, inputs, pkgs, ... }:

{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/scan/not-detected.nix"
    "${inputs.nixpkgs}/nixos/modules/profiles/qemu-guest.nix"
    inputs.disko.nixosModules.disko
    inputs.self.diskoConfigurations.timeweb
    inputs.self.nixosRoles.server
  ];

  deviceSpecific.devInfo.legacy = true;
  boot.loader.grub.device = ""; # or grub.devices will have duplicates

  security.sudo.wheelNeedsPassword = false;

  services.tailscale.enable = true;

  system.stateVersion = "23.11";

  systemd.services = {
    "teledump" = {
      description = "Start telegram account dumper";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      serviceConfig = {
        Restart = "always";
        User = "elxreno";
        Group = "users";

        EnvironmentFile = config.sops.secrets."telegram_bot-env".path;
        Environment = [ "STORE_PATH=~/.teledump" ];

        ExecStart = ''
          ${pkgs.teledump}/bin/teledump
        '';

        KillSignal = "SIGINT";

        Type = "simple";
      };
    };
  };

  services.restic.backups = {
    teledumpbackup = {
      user = "elxreno";
      repository = "rclone:gdrive:teledump-backup";
      passwordFile = config.sops.secrets."restic/teledump".path;
      paths = [
        "/home/elxreno/.teledump"
      ];
      pruneOpts = [
        "--keep-yearly 3"
        "--keep-monthly 2"
        "--keep-weekly 2"
        "--keep-daily 7"
      ];
      timerConfig = {
        OnCalendar = "08:30";
        Persistent = true;
      };
      extraBackupArgs = [
        "--compression=max"
      ];
    };
  };

  sops.secrets."restic/teledump" = {
    owner = config.users.users.elxreno.name;
    restartUnits = [ "restic-backups-teledumpbackup.service" ];
  };

  sops.secrets."telegram_bot-env" = {
    owner = config.users.users.elxreno.name;
  };
}
