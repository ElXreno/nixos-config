{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.bpftune;

  # All tuners except sysctl_tuner. The sysctl_tuner detects systemd-sysctl
  # re-applying static sysctls during nixos-rebuild switch and permanently
  # disables other tuners until bpftune restarts. Since NixOS manages sysctls
  # declaratively, this detection is counterproductive.
  tuners = [
    "ip_frag_tuner"
    "neigh_table_tuner"
    "net_buffer_tuner"
    "netns_tuner"
    "route_table_tuner"
    "tcp_buffer_tuner"
    "tcp_conn_tuner"
    "udp_buffer_tuner"
  ];

  tunerArgs = lib.concatMap (t: [ "-a" "${t}.so" ]) tuners;
in
{
  options.${namespace}.services.bpftune = {
    enable = mkEnableOption "Whether or not to manage bpftune.";
  };

  config = mkIf cfg.enable {
    systemd.services.bpftune = {
      description = "BPF-based auto-tuning of system parameters";
      after = [ "network.target" ];
      wantedBy = [ "multi-user.target" ];
      path = [ pkgs.kmod ];
      serviceConfig = {
        ExecStart = "${lib.getExe pkgs.bpftune} ${lib.escapeShellArgs tunerArgs}";
        Restart = "on-failure";
      };
    };
  };
}
