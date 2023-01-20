{
  services.fail2ban = {
    enable = true;

    jails.DEFAULT =
      ''
        bantime  = 3600
      '';

    jails.sshd =
      ''
        filter = sshd
        maxretry = 4
        action   = iptables[name=ssh, port=ssh, protocol=tcp]
        enabled  = true
      '';
  };

  # Limit stack size to reduce memory usage
  systemd.services.fail2ban.serviceConfig.LimitSTACK = 256 * 1024;
}
