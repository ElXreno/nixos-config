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
            inherit (lib) mkIf;
          in
          {
            ${namespace}.system.impermanence.directories = [
              "/var/lib/tailscale"
            ];

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
              extraUpFlags = [
                "--reset"
              ];
              extraSetFlags = [
                "--accept-dns"
              ];
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

            # https://tailscale.com/docs/reference/best-practices/performance#ethtool-configuration
            services.udev.extraRules = ''
              ACTION=="add", SUBSYSTEM=="net", TEST=="/sys/class/net/%k/device", RUN+="${lib.getExe pkgs.ethtool} -K $name rx-udp-gro-forwarding on rx-gro-list off"
            '';

            systemd.services = {
              tailscaled = {
                before = [ "network.target" ];
                after = [
                  "dnscrypt-proxy.service"
                ];
              };

              tailscaled-autoconnect = {
                serviceConfig.Type = lib.mkForce "exec";
                path = [
                  (pkgs.writeShellScriptBin "systemd-notify" "true")
                ];
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
