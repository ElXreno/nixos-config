_:
let
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

    VLESS server (WS on :10000 behind nginx) and
    sing-box clients (TUN-based) wired together with one VLESS UUID
    per client identity.

    Roles:
    - `server`: terminates WS, fronted by nginx for the
      `datalake.elxreno.com` vhost. Provisions a shared
      `sing-box-uuid-<MACHINE>` for every machine in `roles.client`,
      plus a per-server-only `sing-box-uuid-<NAME>` for each entry
      in `settings.extraClients` (default `[ "mobile" ]`) — those
      UUIDs stay on the server and are copied manually to external
      clients (phones, etc.).
    - `client`: declares its own shared `sing-box-uuid-<HOST>` and
      uses it to dial the server via VLESS-over-WS. `settings.autostart`
      controls whether `sing-box.service` starts at boot.
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

            ${namespace}.services.nginx = {
              enable = true;
              virtualHosts."datalake.elxreno.com" = {
                locations."/api/v1/stream" = {
                  proxyPass = "http://localhost:10000";
                  proxyWebsockets = true;
                  extraConfig = ''
                    proxy_buffering off;
                    proxy_request_buffering off;
                  '';
                };
              };
            };

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
                    listen_port = 10000;

                    inherit users;

                    transport = {
                      type = "ws";
                      path = "/api/v1/stream";
                      max_early_data = 2048;
                      early_data_header_name = "Sec-WebSocket-Protocol";
                      headers.Host = [ "datalake.elxreno.com" ];
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

            users.users.sing-box.extraGroups = [ "acme" ];
          };
      };
  };

  roles.client = {
    description = "Runs the sing-box client (TUN) and tunnels to the server using a per-machine VLESS UUID.";
    interface =
      { lib, ... }:
      {
        options = {
          autostart = lib.mkEnableOption "Whether to execute sing-box client at system boot." // {
            default = true;
          };
          enableSocks = lib.mkEnableOption "Whether to add a socks-proxy as outbound.";
        };
      };

    perInstance =
      { settings, ... }:
      {
        nixosModule =
          {
            config,
            lib,
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
            }
            // lib.optionalAttrs settings.enableSocks {
              sing-box-socks.prompts = {
                address = {
                  description = "IP Address of a SOCKS5 proxy";
                  type = "hidden";
                  persist = true;
                };
                username = {
                  description = "Username of a SOCKS5 proxy";
                  type = "hidden";
                  persist = true;
                };
                password = {
                  description = "Password of a SOCKS5 proxy";
                  type = "hidden";
                  persist = true;
                };
              };
            };

            services.sing-box = {
              enable = true;
              settings = {
                log.level = "debug";

                dns = {
                  strategy = "ipv4_only";
                  servers = [
                    {
                      type = "local";
                    }
                  ];
                };

                inbounds = [
                  {
                    type = "tun";

                    address = [ "172.18.0.1/30" ];

                    mtu = 65535;
                    auto_route = true;
                    auto_redirect = true;
                    strict_route = true;

                    stack = "system";

                    route_address = [
                      "0.0.0.0/0"
                      "::/0"
                    ];

                    exclude_interface = [ "tailscale0" ];

                    route_exclude_address = [
                      "10.0.0.0/8"
                      "172.16.0.0/12"
                      "100.64.0.0/10"

                      # DNS
                      "1.1.1.1/32"
                      "1.0.0.1/32"
                      "8.8.8.8/32"
                      "8.8.4.4/32"

                      # IPv6 loopback
                      "::1/128"

                      # link-local multicast
                      "ff02::/16"
                      # link-local unicast
                      "fe80::/10"
                    ];
                  }
                ];

                outbounds = [
                  {
                    type = "vless";
                    tag = "datalake";

                    server = "datalake.elxreno.com";
                    server_port = 443;

                    uuid._secret = uuidPath;

                    tls = {
                      enabled = true;
                      server_name = "datalake.elxreno.com";
                      utls = {
                        enabled = true;
                        fingerprint = "chrome";
                      };
                    };

                    transport = {
                      type = "ws";
                      path = "/api/v1/stream";
                      max_early_data = 2048;
                      early_data_header_name = "Sec-WebSocket-Protocol";
                      headers.Host = [ "datalake.elxreno.com" ];
                    };
                  }
                  {
                    type = "direct";
                    tag = "direct";
                  }
                ]
                ++ lib.optional settings.enableSocks (
                  with config.clan.core.vars.generators.sing-box-socks.files;
                  {
                    type = "socks";
                    tag = "proxy-seller";
                    server._secret = address.path;
                    server_port = 50101;
                    username._secret = username.path;
                    password._secret = password.path;
                    version = "5";
                  }
                );

                route = {
                  rules = [
                    { action = "sniff"; }
                    {
                      protocol = "dns";
                      action = "hijack-dns";
                    }
                  ]
                  ++ lib.optional settings.enableSocks {
                    domain_suffix = [
                      "anthropic.com"
                      "clau.de"
                      "claude.ai"
                      "claude.com"
                      "claudemcpclient.com"
                      "claudeusercontent.com"
                      "sentry.io"
                      "servd-anthropic-website.b-cdn.net"
                      "statsig.com"
                      "stripe.com"
                      "usefathom.com"
                    ];
                    outbound = "proxy-seller";
                  }
                  ++ [
                    {
                      domain_suffix = [
                        # Arr stack metadata APIs (Cloudflare-blocked)
                        "radarr.video"
                        "sonarr.tv"
                        "servarr.com"
                        "prowlarr.com"

                        # etc
                        "deepl.com"
                        "terraform.io"
                      ];
                      outbound = "datalake";
                    }
                  ];

                  rule_set = [
                    {
                      tag = "geoip-ru";
                      type = "remote";
                      format = "binary";
                      url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs";
                      download_detour = "direct";
                    }
                    {
                      tag = "geoip-by";
                      type = "remote";
                      format = "binary";
                      url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-by.srs";
                      download_detour = "direct";
                    }
                  ];

                  final = "direct";
                  auto_detect_interface = true;
                };

                experimental = {
                  cache_file.enabled = true;
                  clash_api.external_controller = "127.0.0.1:9090";
                  debug.listen = "127.0.0.1:9091";
                };
              };
            };
            systemd.services.sing-box.wantedBy = lib.mkIf (!settings.autostart) (lib.mkForce [ ]);
          };
      };
  };
}
