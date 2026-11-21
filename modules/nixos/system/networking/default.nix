{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    elem
    mkIf
    mkForce
    mkEnableOption
    optionals
    optionalString
    ;
  cfg = config.${namespace}.system.networking;
  wifiDrivers = config.${namespace}.facts.network.wifi.drivers;
  modprobeForDriver = driver: opts: optionalString (elem driver wifiDrivers) opts;
in
{
  options.${namespace}.system.networking = {
    enable = mkEnableOption "managing networking" // {
      default = true;
    };
    networkmanager.enable = mkEnableOption "using NetworkManager" // {
      default = true;
    };
    iwlwifi.larDisable = mkEnableOption "disabling LAR in a rebuilt iwlwifi" // {
      default = elem "iwlwifi" wifiDrivers && !config.${namespace}.facts.network.wifi.iwlwifiMld;
      defaultText = "Whether the WiFi driver is iwlwifi on a card iwlmvm drives.";
    };
    waitOnline = mkEnableOption "holding network-online.target until a link is routable";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence.directories = optionals cfg.networkmanager.enable [
      "/etc/NetworkManager/system-connections"
      "/var/lib/NetworkManager"
    ];

    hardware.facter.detected.dhcp.enable = mkIf cfg.networkmanager.enable false;

    networking = {
      nftables.enable = true;

      networkmanager = {
        inherit (cfg.networkmanager) enable;
        wifi.powersave = true;

        dispatcherScripts = [
          {
            source = pkgs.writeShellScript "configure-rps-hook" ''
              INTERFACE=$1
              ACTION=$2
              LOG_IDENTIFIER=configure-rps-hook

              logger -t $LOG_IDENTIFIER "Event: $ACTION on $INTERFACE"

              if [ "$ACTION" = "up" ] || [ "$ACTION" = "dhcp4-change" ]; then
                if [ ! -d "/sys/class/net/$INTERFACE/queues" ]; then
                   exit 0
                fi

                CPU_COUNT=$(${lib.getExe' pkgs.coreutils "nproc"})

                if [ "$CPU_COUNT" -ge 32 ]; then
                     RPS_MASK="ffffffff"
                else
                     RPS_MASK=$(printf '%x' $(( (1 << CPU_COUNT) - 1 )))
                fi

                FOUND=0
                for rps_file in /sys/class/net/"$INTERFACE"/queues/rx-*/rps_cpus; do
                  if [ -f "$rps_file" ]; then
                    echo "$RPS_MASK" > "$rps_file"
                    FOUND=1
                  fi
                done

                if [ "$FOUND" -eq 1 ]; then
                   logger -t $LOG_IDENTIFIER "Applied RPS mask $RPS_MASK to $INTERFACE"
                fi
              fi
            '';
          }
        ];
      };
    };

    systemd.network.wait-online = {
      enable = mkForce cfg.waitOnline;
      timeout = 30;
    };

    boot = {
      extraModulePackages =
        optionals cfg.iwlwifi.larDisable [
          config.boot.kernelPackages.iwlwifi-lar
        ]
        ++ optionals (builtins.any (d: elem d wifiDrivers) [
          "mt7925e"
          "mt7921e"
        ]) [ config.boot.kernelPackages.mt76-patched ];

      extraModprobeConfig = ''
        options cfg80211 ieee80211_regdom=${if cfg.iwlwifi.larDisable then "US" else "BY"}
      ''
      + modprobeForDriver "iwlwifi" (
        ''
          options iwlwifi amsdu_size=3
        ''
        + optionalString cfg.iwlwifi.larDisable ''
          options iwlwifi lar_disable=1
        ''
        + optionalString (!cfg.networkmanager.enable) ''
          options iwlmvm power_scheme=1
        ''
      )
      + modprobeForDriver "mt7925e" ''
        options mt7925e disable_aspm=1
      '';
    };

    services.udev.extraRules = optionalString (elem "mt7925e" wifiDrivers) ''
      ACTION=="add|bind", SUBSYSTEM=="pci", DRIVER=="mt7925e", ATTR{link/l1_aspm}="0", ATTR{link/l1_1_aspm}="0", ATTR{link/l1_2_aspm}="0", ATTR{link/l1_1_pcipm}="0", ATTR{link/l1_2_pcipm}="0"
    '';
  };
}
