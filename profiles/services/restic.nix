{ config, lib, ... }:

{
  services.restic.backups = lib.mkIf (!config.deviceSpecific.isVM) {
    localbackup = {
      user = "elxreno";
      repository = "/home/elxreno/MEGA/Backups/restic-repo";
      passwordFile = config.sops.secrets."restic/mega_password".path;
      paths = [
        "/home/elxreno"
        "/home/elxreno/Sync/Smartphones/Other/Databases"
      ];
      pruneOpts = [
        "--keep-monthly 2"
        "--keep-weekly 2"
        "--keep-daily 7"
        "--keep-tag payslips"
      ];
      timerConfig = {
        OnCalendar = "08:30";
        Persistent = true;
      };
      extraBackupArgs = [
        "--compression=max"

        "--exclude=/home/elxreno/.android"
        "--exclude=/home/elxreno/.aspnet"
        "--exclude=/home/elxreno/.cache"
        "--exclude=/home/elxreno/.cargo"
        "--exclude=/home/elxreno/.ccache"
        "--exclude=/home/elxreno/.compose-cache"
        "--exclude=/home/elxreno/.config/**/*Cache*"
        "--exclude=/home/elxreno/.config/syncthing/index*"
        "--exclude=/home/elxreno/.continue"
        "--exclude=/home/elxreno/.gitkraken"
        "--exclude=/home/elxreno/.go"
        "--exclude=/home/elxreno/.gradle"
        "--exclude=/home/elxreno/.ipfs"
        "--exclude=/home/elxreno/.java"
        "--exclude=/home/elxreno/.javacpp"
        "--exclude=/home/elxreno/.kde"
        "--exclude=/home/elxreno/.lighthouse"
        "--exclude=/home/elxreno/.local"
        "--exclude=/home/elxreno/.mozilla"
        "--exclude=/home/elxreno/.nethermind"
        "--exclude=/home/elxreno/.nix-defexpr"
        "--exclude=/home/elxreno/.nix-profile"
        "--exclude=/home/elxreno/.npm"
        "--exclude=/home/elxreno/.nuget"
        "--exclude=/home/elxreno/.nuget"
        "--exclude=/home/elxreno/.nv/ComputeCache"
        "--exclude=/home/elxreno/.ollama"
        "--exclude=/home/elxreno/.rustup"
        "--exclude=/home/elxreno/.steam"
        "--exclude=/home/elxreno/.stellarium"
        "--exclude=/home/elxreno/.teledump"
        "--exclude=/home/elxreno/.thunderbird"
        "--exclude=/home/elxreno/.var"
        "--exclude=/home/elxreno/.vscode*"
        "--exclude=/home/elxreno/.wine"
        "--exclude=/home/elxreno/.ZAP"
        "--exclude=/home/elxreno/Android/Sdk"
        "--exclude=/home/elxreno/Android/sources"
        "--exclude=/home/elxreno/backup*"
        "--exclude=/home/elxreno/Calibre Library"
        "--exclude=/home/elxreno/Downloads"
        "--exclude=/home/elxreno/go"
        "--exclude=/home/elxreno/ISOs"
        "--exclude=/home/elxreno/MEGA"
        "--exclude=/home/elxreno/Music"
        "--exclude=/home/elxreno/Pictures"
        "--exclude=/home/elxreno/projects"
        "--exclude=/home/elxreno/rpmbuild"
        "--exclude=/home/elxreno/Sync/Smartphones"
        "--exclude=/home/elxreno/tmp"
        "--exclude=/home/elxreno/Videos"
        "--exclude=/home/elxreno/VMs"
      ];
    };
  };

  sops.secrets."restic/mega_password" = {
    owner = config.users.users.elxreno.name;
    restartUnits = [ "restic-backups-localbackup.service" ];
  };
}
