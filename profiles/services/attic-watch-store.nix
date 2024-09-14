{ config, pkgs, ... }:

let user = "attic-watch-store";
in {
  sops.secrets."attic/watch-store" = { };
  sops.templates."${user}-config" = {
    group = user;
    mode = "440";
    path = "${config.users.users.${user}.home}/.config/attic/config.toml";
    content = ''
      default-server = "local"
      [servers.local]
      endpoint = "http://localhost:8080"
      token = "${config.sops.placeholder."attic/watch-store"}"
    '';
  };

  systemd.services.${user} = {
    description = "Attic watch-store service";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      Type = "simple";
      ExecStart = "${pkgs.attic-client}/bin/attic watch-store local:elxreno";
      Restart = "on-failure";
      RestartSec = "5s";
      StartLimitBurst = 3;
      User = user;
      Group = user;
    };
  };
  users = {
    groups.${user} = { };
    users.${user} = {
      isSystemUser = true;
      home = "/var/lib/${user}";
      createHome = true;
      group = user;
    };
  };
}
