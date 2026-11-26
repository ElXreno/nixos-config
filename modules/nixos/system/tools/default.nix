{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.tools;
in
{
  options.${namespace}.system.tools = {
    enable = mkEnableOption "the base command line and diagnostic tools for every user, root included";
  };

  config = mkIf cfg.enable {
    environment.systemPackages = with pkgs; [
      _7zz
      bc
      binutils
      conntrack-tools
      dmidecode
      dua
      ethtool
      fd
      file
      gdb
      iotop
      iperf3
      jq
      lsof
      ltrace
      mtr
      pciutils
      perf
      pv
      python3
      ripgrep
      smartmontools
      sqlite
      strace
      tcpdump
      tinyxxd
      traceroute
      tree
      unzip
      usbutils
      whois
    ];
  };
}
