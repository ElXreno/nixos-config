{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    optionals
    ;
  cfg = config.${namespace}.system.networking;
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
        enable = cfg.networkmanager.enable;
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
      useDHCP = false;
      useNetworkd = lib.mkDefault false;
    };

    systemd.services.NetworkManager-wait-online.enable = false;
  };
}
