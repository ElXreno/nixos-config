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
    interface =
      { lib, ... }:
      {
        options.domain = lib.mkOption {
          type = lib.types.str;
          description = "Tailnet MagicDNS domain (e.g. angora-ide.ts.net).";
        };
      };
    perInstance =
      {
        mkExports,
        machine,
        settings,
        ...
      }:
      {
        exports = mkExports {
          peer.hosts = [
            { plain = "${lib.toLower machine.name}.${settings.domain}"; }
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
            inherit (lib) mkIf mkOption;
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
                  in
                  "${tsBin} serve --service ${svcName} --bg --yes --https ${port} ${backend}"
                ) svc.endpoints
              ) serveCfg
            );

            declaredServices = lib.concatStringsSep " " (lib.attrNames serveCfg);
            jqBin = lib.getExe pkgs.jq;

            serveScript = pkgs.writeShellScript "tailscale-serve-apply" ''
              current=$(${tsBin} serve status --json | ${jqBin} -r '.Services // {} | keys[]')
              for svc in $current; do
                if ! echo "${declaredServices}" | grep -qw "$svc"; then
                  ${tsBin} serve clear "$svc"
                fi
              done

              ${lib.concatStringsSep "\n" serveCommands}
            '';
          in
          {
            options.${namespace}.tailscale.serve = mkOption {
              type = lib.types.attrsOf (
                lib.types.submodule {
                  options.endpoints = mkOption {
                    type = lib.types.attrsOf lib.types.str;
                    default = { };
                    example = {
                      "tcp:443" = "http://localhost:8080";
                    };
                    description = ''
                      Mapping of protocol:port to local backend URL.
                      Tailscale terminates TLS and proxies to the backend.
                    '';
                  };
                }
              );
              default = { };
              description = ''
                Tailscale Services to expose from this machine. Each key is a
                service name (e.g. "svc:radarr") that must exist in the tailnet
                (created via OpenTofu or the admin console). The endpoints map
                the service's ports to local backends.
              '';
            };

            config = {
              ${namespace}.system.impermanence.directories = [ "/var/lib/tailscale" ];

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
                permitCertUid = with config.services.caddy; mkIf enable user;
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
                  before = [ "network.target" ];
                  after = [ "dnscrypt-proxy.service" ];
                };

                tailscaled-autoconnect = {
                  serviceConfig.Type = lib.mkForce "exec";
                  path = [ (pkgs.writeShellScriptBin "systemd-notify" "true") ];
                };

                tailscale-serve = {
                  description = "Apply Tailscale serve configuration";
                  after = [
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
