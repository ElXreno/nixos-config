{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    concatMapStringsSep
    concatStringsSep
    filter
    listToAttrs
    mkIf
    mkOption
    nameValuePair
    optionalString
    types
    ;
  cfg = config.${namespace}.system.networking;
  names = cfg.routes.cgroups;
  wireguard = filter (name: cfg.tunnels.${name}.type == "wireguard") names;

  ip = "${pkgs.iproute2}/bin/ip";
  nft = "${pkgs.nftables}/bin/nft";

  match = name: "add rule inet via output socket cgroupv2 level 2 @${name}";

  rules = pkgs.writeText "via.nft" ''
    add table inet via
    ${concatMapStringsSep "\n" (name: "add set inet via ${name} { type cgroupsv2; }") names}
    add chain inet via output { type route hook output priority mangle; policy accept; }
    flush chain inet via output
    add rule inet via output oifname "lo" return
    ${concatMapStringsSep "\n" (
      name: "${match name} meta mark set ${toString cfg.tunnels.${name}.hostAccess.mark}"
    ) wireguard}
  '';

  backstop = concatMapStringsSep "\n" (
    name:
    let
      inherit (cfg.tunnels.${name}.hostAccess) mark table;
      rule = "fwmark ${toString mark} prohibit priority ${toString (table + 1000)}";
    in
    ''
      for family in -4 -6; do
        ${ip} $family rule del ${rule} 2>/dev/null || true
        ${ip} $family rule add ${rule}
      done
    ''
  ) wireguard;

  setup = pkgs.writeShellScript "via-routes" ''
    set -e
    ${nft} -f ${rules}
    ${backstop}
  '';

  attach = pkgs.writeShellScript "via-attach" ''
    set -eu
    tunnel=$1
    pid=$2
    case " ${concatStringsSep " " names} " in
      *" $tunnel "*) ;;
      *)
        echo "via: unknown tunnel $tunnel, known: ${concatStringsSep " " names}" >&2
        exit 64
        ;;
    esac
    case $pid in
      "" | *[!0-9]*) exit 64 ;;
    esac
    if [ "$(${pkgs.coreutils}/bin/stat -c %u "/proc/$pid")" != "''${SUDO_UID:-0}" ]; then
      echo "via: process $pid belongs to another user" >&2
      exit 1
    fi
    ${optionalString (wireguard != [ ]) ''
      case " ${concatStringsSep " " wireguard} " in
        *" $tunnel "*)
          if ! ${config.systemd.package}/bin/systemctl is-active --quiet "$tunnel.service"; then
            echo "via: tunnel $tunnel is not running" >&2
            exit 1
          fi
          ;;
      esac
    ''}
    ${config.systemd.package}/bin/busctl call --quiet org.freedesktop.systemd1 /org/freedesktop/systemd1 \
      org.freedesktop.systemd1.Manager StartTransientUnit 'ssa(sv)a(sa(sv))' \
      "via-$tunnel-$pid.scope" fail 2 PIDs au 1 "$pid" Slice s "via-$tunnel.slice" 0
    for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
      if ${pkgs.gnugrep}/bin/grep -q "/via-$tunnel.slice/" "/proc/$pid/cgroup" \
        && ${nft} list set inet via "$tunnel" | ${pkgs.gnugrep}/bin/grep -q elements; then
        exit 0
      fi
      ${pkgs.coreutils}/bin/sleep 0.1
    done
    echo "via: traffic of the $tunnel slice is not marked, refusing to start" >&2
    exit 1
  '';

  via = pkgs.writeShellApplication {
    name = "via";
    text = ''
      if [ $# -lt 2 ]; then
        echo "usage: via <${concatStringsSep "|" names}> <command> [argument...]" >&2
        exit 64
      fi
      tunnel=$1
      shift
      /run/wrappers/bin/sudo -n ${attach} "$tunnel" $$
      exec "$@"
    '';
  };
in
{
  options.${namespace}.system.networking.routes = {
    cgroups = mkOption {
      type = types.listOf types.str;
      default = [ ];
      example = [ "home" ];
      description = "Tunnels a single program can be started through with `via <tunnel> <command>`.";
    };
  };

  config = mkIf (names != [ ]) {
    environment.systemPackages = [ via ];

    security.sudo.extraRules = [
      {
        groups = [ "wheel" ];
        commands = [
          {
            command = "${attach}";
            options = [ "NOPASSWD" ];
          }
        ];
      }
    ];

    systemd.services.via-routes = {
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-pre.target" ];
      before = [ "network-pre.target" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecStart = setup;
      };
    };

    systemd.slices = listToAttrs (
      map (
        name:
        nameValuePair "via-${name}" {
          requires = [ "via-routes.service" ];
          after = [ "via-routes.service" ];
          sliceConfig.NFTSet = "cgroup:inet:via:${name}";
        }
      ) names
    );
  };
}
