_:
let
  serverDomain = "datalake.elxreno.com";
  hysteria2Port = 10001;

  uuidGenerator = {
    files.uuid.secret = true;
    script = ''
      cat /proc/sys/kernel/random/uuid > "$out/uuid"
    '';
  };
in
{
  _class = "clan.service";
  manifest.name = "sing-box";
  manifest.description = "VLESS proxy with per-machine client identities for the clan.";
  manifest.categories = [ "Network" ];
  manifest.readme = ''
    # sing-box

    VLESS and hysteria2 server (WS on :10000 behind caddy) with one
    UUID per client identity; clients route a list of domains to it.

    Roles:
    - `server`: terminates WS, fronted by caddy for the
      `datalake.elxreno.com` vhost. Provisions a shared
      `sing-box-uuid-<MACHINE>` for every machine in `roles.client`,
      plus a per-server-only `sing-box-uuid-<NAME>` for each entry
      in `settings.extraClients` (default `[ "mobile" ]`) — those
      UUIDs stay on the server and are copied manually to external
      clients (phones, etc.).
    - `client`: declares its own shared `sing-box-uuid-<HOST>`, a
      hysteria2 tunnel `home` and the domains routed through it.
      `settings.autostart` controls whether the domain router starts
      at boot.
  '';

  roles.server = {
    description = "Terminates VLESS clients and provisions a UUID per client machine + extra identity.";
    interface =
      { lib, ... }:
      {
        options.extraClients = lib.mkOption {
          type = lib.types.listOf lib.types.str;
          default = [ "mobile" ];
          description = "Extra non-clan client identities to provision UUIDs for (e.g. phones).";
        };

      };

    perInstance =
      { roles, settings, ... }:
      {
        nixosModule =
          {
            config,
            lib,
            namespace,
            ...
          }:
          let
            inherit (lib)
              attrNames
              elem
              listToAttrs
              ;

            clientMachines = attrNames (roles.client.machines or { });
            allClients = clientMachines ++ settings.extraClients;
            uuidPath = name: config.clan.core.vars.generators."sing-box-uuid-${name}".files.uuid.path;
            users = map (name: {
              inherit name;
              uuid._secret = uuidPath name;
            }) allClients;
          in
          {
            clan.core.vars.generators = listToAttrs (
              map (name: {
                name = "sing-box-uuid-${name}";
                value = uuidGenerator // {
                  share = elem name clientMachines;
                };
              }) allClients
            );

            ${namespace} = {
              ports.sing-box-vless = 10000;
              ports.sing-box-hysteria2 = hysteria2Port;
              system.networking.firewall.public.hysteria2.udp = [ hysteria2Port ];

              services.caddy.enable = true;
            };

            services.caddy.virtualHosts.${serverDomain}.extraConfig = ''
              handle /api/v1/stream {
                reverse_proxy localhost:${toString config.${namespace}.ports.sing-box-vless} {
                  flush_interval -1
                }
              }
              handle {
                respond 404
              }
            '';

            systemd.services.sing-box.serviceConfig.Nice = -10;

            services.sing-box = {
              enable = true;
              settings = {
                log.level = "debug";

                dns = {
                  servers = [
                    {
                      type = "https";
                      tag = "dns-direct";
                      server = "1.1.1.1";
                    }
                  ];
                  final = "dns-direct";
                  strategy = "prefer_ipv6";
                };

                outbounds = [
                  {
                    tag = "direct-out";
                    type = "direct";
                  }
                ];

                inbounds = [
                  {
                    tag = "vless-in";
                    type = "vless";

                    listen = "::";
                    listen_port = config.${namespace}.ports.sing-box-vless;

                    inherit users;

                    transport = {
                      type = "ws";
                      path = "/api/v1/stream";
                      max_early_data = 2048;
                      early_data_header_name = "Sec-WebSocket-Protocol";
                      headers.Host = [ serverDomain ];
                    };
                  }
                  {
                    tag = "hysteria2-in";
                    type = "hysteria2";

                    listen = "::";
                    listen_port = config.${namespace}.ports.sing-box-hysteria2;

                    users = map (name: {
                      inherit name;
                      password._secret = uuidPath name;
                    }) allClients;

                    tls = {
                      enabled = true;
                      server_name = serverDomain;
                      certificate_path = "${config.security.acme.certs.${serverDomain}.directory}/fullchain.pem";
                      key_path = "${config.security.acme.certs.${serverDomain}.directory}/key.pem";
                    };
                  }
                ];

                route = {
                  rules = [
                    { action = "sniff"; }
                    {
                      domain_suffix = [
                        ".ru"
                        ".by"
                      ];
                      rule_set = [
                        "geoip-ru"
                        "geoip-by"
                      ];
                      action = "reject";
                    }
                    {
                      protocol = "bittorrent";
                      action = "reject";
                    }
                  ];

                  rule_set = [
                    {
                      tag = "geoip-ru";
                      type = "remote";
                      format = "binary";
                      url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs";
                    }
                    {
                      tag = "geoip-by";
                      type = "remote";
                      format = "binary";
                      url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-by.srs";
                    }
                  ];

                  final = "direct-out";
                  auto_detect_interface = true;
                };
              };
            };

            users.users.sing-box.extraGroups = [
              "acme"
              "caddy"
            ];
          };
      };
  };

  roles.client = {
    description = "Routes the listed domains to the server over hysteria2 with a per-machine identity.";
    interface =
      { lib, ... }:
      {
        options = {
          autostart = lib.mkEnableOption "starting the domain router at system boot" // {
            default = true;
          };
        };
      };

    perInstance =
      { settings, ... }:
      {
        nixosModule =
          {
            config,
            namespace,
            ...
          }:
          let
            inherit (config.networking) hostName;
            uuidPath = config.clan.core.vars.generators."sing-box-uuid-${hostName}".files.uuid.path;
          in
          {
            clan.core.vars.generators = {
              "sing-box-uuid-${hostName}" = uuidGenerator // {
                share = true;
              };
            };

            ${namespace}.system.networking = {
              tunnels.home = {
                type = "hysteria2";
                server = serverDomain;
                port = hysteria2Port;
                passwordFile = uuidPath;
              };

              routes = {
                inherit (settings) autostart;

                cgroups = [ "home" ];

                domains.home = [
                  # enabled for the arr stack metadata APIs
                  "radarr.video"
                  "sonarr.tv"
                  "servarr.com"
                  "prowlarr.com"

                  "github.com"
                  "githubassets.com"
                  "githubusercontent.com"

                  "scdn.co"
                  "spotify.com"
                  "spotifycdn.com"
                  "spotifycdn.net"
                  "akamaized.net"

                  "discord.com"
                  "discord.gg"
                  "discord.media"
                  "discordapp.com"
                  "discordapp.net"

                  "chat.com"
                  "chatgpt.com"
                  "crixet.com"
                  "oaistatic.com"
                  "oaistatsig.com"
                  "oaiusercontent.com"
                  "openai.com"
                  "sora.com"

                  "anthropic.com"
                  "clau.de"
                  "claude.ai"
                  "claude.com"
                  "claudemcpclient.com"
                  "claudeusercontent.com"
                  "servd-anthropic-website.b-cdn.net"

                  "deepl.com"
                  "fabricmc.net"
                  "modrinth.com"
                  "terraform.io"
                  "grafana.com"
                  "patreon.com"
                  "statsig.com"
                  "stripe.com"
                  "usefathom.com"
                  "sentry.io"
                  "quay.io"
                  "qualcomm.com"
                  "fragment.com"

                  "geosite:microsoft"
                ];
              };
            };
          };
      };
  };
}
