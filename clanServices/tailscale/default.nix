{
  clanLib,
  config,
  lib,
  ...
}:
{
  _class = "clan.service";
  manifest.name = "tailscale";
  manifest.description = "Tailscale mesh VPN with optional exit-node advertisement";
  manifest.categories = [
    "System"
    "Network"
  ];
  manifest.readme = ''
    Roles:
    - `node`: applied to every machine. Connects to the tailnet, trusts the
      interface, tunes networking, exports the machine's MagicDNS FQDN.
      Supports `serve` option for exposing local services via Tailscale Services.
    - `exit-node`: marker role. Machines with this role advertise as exit nodes.
  '';
  manifest.exports.out = [
    "networking"
    "peer"
  ];

  exports = lib.mapAttrs' (instanceName: _: {
    name = clanLib.buildScopeKey {
      inherit instanceName;
      serviceName = config.manifest.name;
    };
    value = {
      networking.priority = 1500;
    };
  }) config.instances;

  roles.node = {
    description = "Connects the machine to the tailnet, trusts the interface, tunes networking.";
    perInstance =
      {
        mkExports,
        machine,
        meta,
        ...
      }:
      {
        exports = mkExports {
          peer.hosts = [
            { plain = "${lib.toLower machine.name}.${meta.domain}"; }
          ];
        };
        nixosModule =
          {
            config,
            pkgs,
            lib,
            namespace,
            ...
          }:
          let
            serveCfg = config.${namespace}.tailscale.serve;
            tsBin = lib.getExe config.services.tailscale.package;
            # TODO: switch to `tailscale serve set-config` when upstream fixes
            # the HTTPS frontend + HTTP backend round-trip bug:
            # https://github.com/tailscale/tailscale/issues/18381
            serveCommands = lib.concatLists (
              lib.mapAttrsToList (
                svcName: svc:
                lib.mapAttrsToList (
                  portSpec: backend:
                  let
                    port = lib.last (lib.splitString ":" portSpec);
                    appCaps = lib.optionalString (
                      svc.acceptAppCaps != [ ]
                    ) "--accept-app-caps ${lib.concatStringsSep "," svc.acceptAppCaps} ";
                  in
                  "${tsBin} serve --service ${svcName} --bg --yes ${appCaps}--https ${port} ${backend}"
                ) svc.endpoints
              ) serveCfg
            );

            nodeCfg = config.${namespace}.tailscale.serveNode;
            nodePort = portSpec: lib.last (lib.splitString ":" portSpec);
            nodeCommands = lib.mapAttrsToList (
              portSpec: entry:
              let
                appCaps = lib.optionalString (
                  entry.acceptAppCaps != [ ]
                ) "--accept-app-caps ${lib.concatStringsSep "," entry.acceptAppCaps} ";
              in
              "${tsBin} serve --bg --yes ${appCaps}--https ${nodePort portSpec} ${entry.backend}"
            ) nodeCfg;
            declaredPorts = lib.concatStringsSep " " (map nodePort (lib.attrNames nodeCfg));

            declaredServices = lib.concatStringsSep " " (lib.attrNames serveCfg);
            jqBin = lib.getExe pkgs.jq;

            serveScript = pkgs.writeShellScript "tailscale-serve-apply" ''
              current=$(${tsBin} serve status --json | ${jqBin} -r '.Services // {} | keys[]')
              for svc in $current; do
                if ! echo "${declaredServices}" | grep -qw "$svc"; then
                  ${tsBin} serve clear "$svc"
                fi
              done

              ports=$(${tsBin} serve status --json | ${jqBin} -r '.TCP // {} | keys[]')
              for port in $ports; do
                if ! echo "${declaredPorts}" | grep -qw "$port"; then
                  ${tsBin} serve --https="$port" off
                fi
              done

              ${lib.concatStringsSep "\n" (serveCommands ++ nodeCommands)}
            '';
          in
          {
            config = {
              ${namespace}.system = {
                impermanence.directories = [ "/var/lib/tailscale" ];
                networking.firewall.public.tailscale = {
                  upstream = "nixos/modules/services/networking/tailscale.nix";
                  udp = [ config.services.tailscale.port ];
                };
              };

              clan.core.vars.generators.tailscale = {
                prompts.authKey = {
                  description = ''
                    Provide a tailscale "auth key" to connect to the desired network.
                    See <https://login.tailscale.com/admin/settings/keys>.
                  '';
                  type = "line";
                };

                files.authKey.secret = true;

                script = ''
                  cat $prompts/authKey > $out/authKey
                '';
              };

              services.tailscale = {
                enable = true;
                authKeyFile = config.clan.core.vars.generators.tailscale.files.authKey.path;
                openFirewall = true;
                useRoutingFeatures = "both";
                extraUpFlags = [ "--reset" ];
                extraSetFlags = [ "--accept-dns" ];
              };

              networking =
                let
                  ts = config.services.tailscale;
                in
                {
                  firewall.trustedInterfaces = [ ts.interfaceName ];
                  networkmanager.unmanaged = [ ts.interfaceName ];
                };

              services.udev.extraRules = ''
                ACTION=="add", SUBSYSTEM=="net", TEST=="/sys/class/net/%k/device", RUN+="${lib.getExe pkgs.ethtool} -K $name rx-udp-gro-forwarding on rx-gro-list off"
              '';

              systemd.services = {
                tailscaled = {
                  after = [ "dnscrypt-proxy.service" ];
                  serviceConfig.Nice = -10;
                };

                tailscaled-autoconnect = {
                  after = [ "multi-user.target" ];
                  serviceConfig = {
                    Restart = "on-failure";
                    RestartSec = "10s";
                  };
                };

                tailscaled-set.after = [ "multi-user.target" ];

                tailscale-serve = {
                  description = "Apply Tailscale serve configuration";
                  after = [
                    "multi-user.target"
                    "tailscaled.service"
                    "tailscaled-autoconnect.service"
                  ];
                  wants = [ "tailscaled-autoconnect.service" ];
                  wantedBy = [ "multi-user.target" ];
                  serviceConfig = {
                    Type = "oneshot";
                    RemainAfterExit = true;
                    ExecStartPre = "${tsBin} wait";
                    ExecStart = serveScript;
                    TimeoutStartSec = "5min";
                  };
                };
              };
            };
          };
      };
  };

  roles.exit-node = {
    description = "Advertises this machine as a Tailscale exit node.";
    perInstance = _: {
      nixosModule = _: {
        services.tailscale.extraSetFlags = [ "--advertise-exit-node" ];
      };
    };
  };
}
