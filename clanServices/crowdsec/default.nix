_:
let
  mkMachineGenerator =
    {
      pkgs,
      lapiUrl,
      machineName,
    }:
    {
      share = true;
      files = {
        password.secret = true;
        bouncer-key.secret = true;
        "lapi-credentials.yaml".secret = true;
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        password=$(openssl rand -hex 32 | tr -d '\n')
        bouncer_key=$(openssl rand -hex 32 | tr -d '\n')

        echo -n "$password" > "$out/password"
        echo -n "$bouncer_key" > "$out/bouncer-key"

        cat > "$out/lapi-credentials.yaml" <<EOF
        url: ${lapiUrl}
        login: ${machineName}
        password: $password
        EOF
      '';
    };
in
{
  _class = "clan.service";
  manifest.name = "crowdsec";
  manifest.description = "CrowdSec IDS/IPS with a central LAPI and distributed agents/bouncers";
  manifest.categories = [
    "System"
    "Network"
    "Security"
  ];
  manifest.readme = ''
    # crowdsec

    Deploys a shared CrowdSec fleet across clan machines with one
    central LAPI and any number of agent/bouncer clients.

    Every machine runs the crowdsec agent (parsing local logs and
    reporting alerts) and the firewall bouncer (enforcing ban
    decisions via nftables/iptables). Ban decisions are stored on
    the LAPI server and pulled by all bouncers, so a scraper seen
    on any machine is blocked on all of them.

    Roles:
    - `client`: applied to every machine (including the server).
      Configures the agent, bouncer, hub collections, acquisitions
      and whitelists. Each machine declares its own shared
      `crowdsec-machine-<name>` clan vars generator with a random
      password and bouncer API key.
    - `server`: applied to exactly one machine in addition to
      `client`. Enables the LAPI, makes it listen on every
      interface (Tailscale trustedInterfaces limits exposure),
      registers with the CrowdSec Central API (CAPI) for the
      community blocklist, and runs an ExecStartPre script that
      registers every client machine and bouncer with the
      pre-generated credentials.

    The server machine talks to its own LAPI via its Tailscale
    hostname, which resolves back to itself via the kernel
    loopback fast-path, so no URL special-casing is needed.
  '';

  roles.client = {
    description = "Runs the agent and firewall bouncer on every machine.";
    perInstance =
      { roles, ... }:
      {
        nixosModule =
          {
            config,
            pkgs,
            lib,
            namespace,
            ...
          }:
          let
            inherit (lib)
              optional
              optionals
              ;

            inherit (config.networking) hostName;
            nginxEnabled = config.services.nginx.enable;

            serverName = lib.head (lib.attrNames (roles.server.machines or { }));
            isServer = serverName == hostName;

            inherit (config.clan.core.settings) domain;
            lapiListenPort = "8080";
            lapiHost = "${lib.toLower serverName}.${domain}";
            lapiUrl = "http://${lapiHost}:${lapiListenPort}";
          in
          {
            ${namespace}.system.impermanence.directories = [
              {
                directory = "/var/lib/private/crowdsec";
                user = "crowdsec";
                group = "crowdsec";
                mode = "0750";
              }
            ];

            clan.core.vars.generators = lib.optionalAttrs (!isServer) {
              "crowdsec-machine-${hostName}" = mkMachineGenerator {
                inherit pkgs lapiUrl;
                machineName = hostName;
              };
            };

            services.crowdsec = {
              enable = true;
              autoUpdateService = true;

              extraGroups = [ "systemd-journal" ] ++ optional nginxEnabled "nginx";
              readOnlyPaths = optional nginxEnabled "/var/log/nginx";

              hub = {
                collections = [
                  "crowdsecurity/linux"
                  "crowdsecurity/sshd"
                ]
                ++ optionals nginxEnabled [
                  "crowdsecurity/nginx"
                  "crowdsecurity/http-cve"
                  "crowdsecurity/base-http-scenarios"
                  "crowdsecurity/wordpress"
                ];
                parsers = [ "crowdsecurity/whitelists" ];
              };

              settings = {
                acquisitions = [
                  {
                    source = "journalctl";
                    journalctl_filter = [ "_SYSTEMD_UNIT=sshd.service" ];
                    labels.type = "syslog";
                  }
                ]
                ++ optional nginxEnabled {
                  source = "file";
                  filenames = [ "/var/log/nginx/access.log" ];
                  labels.type = "nginx";
                };

                parsers.s02Enrich = [
                  {
                    name = "local/whitelist-tailscale";
                    description = "Whitelist Tailscale CGNAT (IPv4) and ULA (IPv6) ranges";
                    whitelist = {
                      reason = "Internal clan Tailscale traffic";
                      cidr = [
                        "100.64.0.0/10"
                        "fd7a:115c:a1e0::/48"
                      ];
                    };
                  }
                  {
                    name = "local/whitelist-common-404s";
                    description = "Don't treat harmless browser-requested 404s as probing";
                    whitelist = {
                      reason = "Common files browsers request that often don't exist";
                      expression = [
                        "evt.Parsed.request matches '^/favicon[.]ico([?].*)?$'"
                      ];
                    };
                  }
                  {
                    name = "local/whitelist-nix-cache-paths";
                    description = "Don't treat Nix cache 404s as probing";
                    whitelist = {
                      reason = "Nix narinfo/nar lookups naturally 404, not scanner behavior";
                      expression = [
                        "evt.Parsed.request matches '[.]nar(info|[.](xz|zst|bz2|gz))?([?].*)?$'"
                      ];
                    };
                  }
                  {
                    name = "local/whitelist-matrix-paths";
                    description = "Don't treat normal Matrix client 4xx noise as bruteforce";
                    whitelist = {
                      reason = "Matrix clients legitimately receive 4xx on many endpoints (federation-relayed publicRooms 403s, missing account_data/room_keys, MAS-disabled /v3/login 404s, msc3266 previews); real auth bruteforce surface lives on the MAS vhost, not /_matrix/";
                      expression = [
                        "evt.Parsed.request matches '^/_(matrix|synapse)/'"
                      ];
                    };
                  }
                ];

                config = {
                  cscli.output = "human";
                  api = lib.optionalAttrs (!isServer) {
                    client.credentials_path = "/run/credentials/crowdsec.service/lapi-credentials.yaml";
                    server.enable = false;
                  };
                };
              };
            };

            services.crowdsec-firewall-bouncer = {
              enable = true;
              registerBouncer.enable = isServer;
              secrets.apiKeyPath = lib.mkIf (
                !isServer
              ) config.clan.core.vars.generators."crowdsec-machine-${hostName}".files.bouncer-key.path;
              settings = {
                api_url = lib.mkDefault lapiUrl;
                mode = if config.networking.nftables.enable then "nftables" else "iptables";
                log_mode = "stdout";
                update_frequency = "10s";
              };
            };

            systemd.services = {
              crowdsec = {
                after = lib.optional config.${namespace}.services.dnscrypt-proxy.enable "dnscrypt-proxy.service";
                wants = lib.optional config.${namespace}.services.dnscrypt-proxy.enable "dnscrypt-proxy.service";
                serviceConfig = {
                  LoadCredential =
                    lib.optional (!isServer)
                      "lapi-credentials.yaml:${
                        config.clan.core.vars.generators."crowdsec-machine-${hostName}".files."lapi-credentials.yaml".path
                      }";
                  # nixpkgs runs `crowdsec -t -error` as pre-start, which does
                  # the same full initialization (regex compile, GeoIP load) as
                  # the daemon itself. On slow CPUs this doubles startup time.
                  ExecStartPre = lib.mkForce [ ];
                  TimeoutStartSec = 300;
                };
              };

              crowdsec-setup = lib.mkIf config.${namespace}.services.dnscrypt-proxy.enable {
                after = [ "dnscrypt-proxy.service" ];
                wants = [ "dnscrypt-proxy.service" ];
              };
            };
          };
      };
  };

  roles.server = {
    description = "Runs the LAPI and registers every client machine + bouncer.";
    perInstance =
      { roles, ... }:
      {
        nixosModule =
          {
            config,
            pkgs,
            lib,
            ...
          }:
          let
            inherit (lib)
              mkAfter
              listToAttrs
              concatMap
              concatMapStringsSep
              getExe'
              ;

            inherit (config.networking) hostName;
            clientMachineNames = lib.attrNames (roles.client.machines or { });
            remoteClientNames = lib.filter (n: n != hostName) clientMachineNames;

            inherit (config.clan.core.settings) domain;
            lapiListenPort = "8080";
            lapiHost = "${lib.toLower hostName}.${domain}";
            lapiUrl = "http://${lapiHost}:${lapiListenPort}";

            dataDir = config.services.crowdsec.settings.config.config_paths.data_dir;
          in
          {
            services.crowdsec.settings = {
              config.api.server = {
                enable = true;
                listen_uri = "0.0.0.0:${lapiListenPort}";
                online_client.credentials_path = "${dataDir}/online_api_credentials.yaml";
              };
              console.enrollKeyFile = config.clan.core.vars.generators.crowdsec-console-token.files.token.path;
            };

            services.crowdsec-firewall-bouncer.settings.api_url = "http://127.0.0.1:${lapiListenPort}";

            clan.core.vars.generators =
              listToAttrs (
                map (name: {
                  name = "crowdsec-machine-${name}";
                  value = mkMachineGenerator {
                    inherit pkgs lapiUrl;
                    machineName = name;
                  };
                }) remoteClientNames
              )
              // {
                crowdsec-console-token = {
                  prompts.token = {
                    description = "CrowdSec Console enrollment token.";
                    type = "hidden";
                    persist = true;
                  };
                };
              };

            systemd.services.crowdsec.serviceConfig = {
              LoadCredential = concatMap (name: [
                "${name}-password:${
                  config.clan.core.vars.generators."crowdsec-machine-${name}".files.password.path
                }"
                "${name}-bouncer-key:${
                  config.clan.core.vars.generators."crowdsec-machine-${name}".files.bouncer-key.path
                }"
              ]) remoteClientNames;

              ExecStartPre = mkAfter [
                (pkgs.writeShellScript "crowdsec-register" ''
                  set -eu
                  set -o pipefail

                  cscli=${getExe' config.services.crowdsec.package "cscli"}
                  cat=${getExe' pkgs.coreutils "cat"}
                  grep=${getExe' pkgs.gnugrep "grep"}

                  ${concatMapStringsSep "\n" (name: ''
                    password="$("$cat" "$CREDENTIALS_DIRECTORY/${name}-password")"
                    bouncerKey="$("$cat" "$CREDENTIALS_DIRECTORY/${name}-bouncer-key")"

                    "$cscli" machines delete "${name}" 2>/dev/null || true
                    "$cscli" machines add "${name}" --password "$password" -f /dev/null

                    if "$cscli" bouncers list | "$grep" -q "${name}-bouncer"; then
                      "$cscli" bouncers delete "${name}-bouncer" || true
                    fi
                    "$cscli" bouncers add "${name}-bouncer" --key "$bouncerKey"
                  '') remoteClientNames}
                '')
              ];
            };
          };
      };
  };
}
