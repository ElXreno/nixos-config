_: {
  services.fail2ban = {
    enable = true;

    bantime = "3h";
    bantime-increment = {
      enable = true;
      rndtime = "15m";
    };

    jails = {
      nginx-general.settings = {
        enabled = true;
        port = "http,https";
        protocol = "tcp,udp";
        filter = "nginx-general";
        logpath = "%(nginx_access_log)s";
        backend = "auto";
      };
    };
  };

  environment.etc."fail2ban/filter.d/nginx-general.conf".text = ''
    [Definition]
    failregex = ^<HOST> - .* "(GET|POST|HEAD)(?! \/(favicon\.ico|.*\/.*\.nar(info)?)) .*" (404|444|403|400) .*$
  '';
}
