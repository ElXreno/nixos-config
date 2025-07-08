{ pkgs, ... }:
{
  services.fail2ban = {
    enable = true;

    bantime = "3h";
    bantime-increment = {
      enable = true;
      rndtime = "15m";
    };

    jails = {
      nginx-botsearch.settings.enabled = true;
      nginx-bad-request.settings.enabled = true;
      nginx-forbidden.settings.enabled = true;
    };

    # Possibly useless as nginx stores real IP instead of proxied IP.
    ignoreIP = pkgs.cfipv4;
  };
}
