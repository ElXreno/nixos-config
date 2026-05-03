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
    enable = mkEnableOption "Whether or not to manage networking." // {
      default = true;
    };
    networkmanager.enable = mkEnableOption "Whether to use NetworkManager." // {
      default = !config.${namespace}.roles.server.enable;
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence = {
      directories = optionals cfg.networkmanager.enable [
        "/etc/NetworkManager/system-connections"
      ];
    };

    # for some reason *this* is what makes networkmanager not get screwed completely instead of the impermanence module
    systemd.tmpfiles.rules = optionals cfg.networkmanager.enable (
      with config.${namespace}.system.impermanence;
      [
        "L /var/lib/NetworkManager/secret_key - - - - ${defaultPersistentPath}/var/lib/NetworkManager/secret_key"
        "L /var/lib/NetworkManager/seen-bssids - - - - ${defaultPersistentPath}/var/lib/NetworkManager/seen-bssids"
        "L /var/lib/NetworkManager/timestamps - - - - ${defaultPersistentPath}/var/lib/NetworkManager/timestamps"
      ]
    );

    networking = {
      networkmanager = {
        inherit (cfg.networkmanager) enable;
        wifi.powersave = false;

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

    systemd.services.NetworkManager-wait-online.enable = false;

    systemd.network.wait-online = {
      # Override clan-core behavior
      enable = mkForce config.${namespace}.roles.server.enable;
      timeout = 30;
    };

    boot = {
      extraModulePackages =
        optionals (builtins.elem "iwlwifi" config.${namespace}.facts.network.wifi.drivers) [
          config.boot.kernelPackages.iwlwifi-lar
        ]
        ++
          optionals
            (builtins.any (d: builtins.elem d config.${namespace}.facts.network.wifi.drivers) [
              "mt7925e"
              "mt7921e"
            ])
            [
              config.boot.kernelPackages.mt76-tdls-fix
            ];

      extraModprobeConfig = ''
        options cfg80211 ieee80211_regdom=US
      ''
      + modprobeForDriver "iwlwifi" ''
        options iwlwifi amsdu_size=3
        options iwlwifi lar_disable=1
        options iwlmvm power_scheme=1
        options iwldvm force_cam=Y
      ''
      + modprobeForDriver "mt7925e" ''
        options mt7925e disable_aspm=1
      ''
      + modprobeForDriver "mt7921e" ''
        options mt7921e disable_aspm=1
      '';
    };
  };
}
