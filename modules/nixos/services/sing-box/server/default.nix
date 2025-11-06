{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    listToAttrs
    nameValuePair
    ;
  cfg = config.${namespace}.services.sing-box.server;
in
{
  options.${namespace}.services.sing-box.server = {
    enable = mkEnableOption "Whether or not to manage sing-box server.";
    clients = mkOption {
      type = types.listOf types.str;
      default = [
        namespace
        "mobile"
        "noncere"
      ];
    };
  };

  config = mkIf cfg.enable {
    sops = {
      secrets = {
        "hysteria2/obfs-salamander-password" = {
          owner = "sing-box";
          group = "sing-box";
        };
      }
      // listToAttrs (
        map (
          client:
          nameValuePair "trojan/${client}-password" {
            owner = "sing-box";
            group = "sing-box";
          }
        ) cfg.clients
      )
      // listToAttrs (
        map (
          client:
          nameValuePair "hysteria2/${client}-password" {
            owner = "sing-box";
            group = "sing-box";
          }
        ) cfg.clients
      );
    };

    networking.firewall = {
      allowedTCPPorts = [
        443
        6443
      ];
      allowedUDPPorts = [
        443
        6443
      ];
    };

    services.sing-box = {
      enable = true;
      settings = {
        log.level = "debug";
        dns = {
          servers = [
            {
              tag = "local";
              type = "local";
            }
          ];
          final = "local";
          strategy = "prefer_ipv6";
        };

        route = {
          final = "direct-out";
          auto_detect_interface = true;
        };

        outbounds = [
          {
            tag = "direct-out";
            type = "direct";
          }
        ];

        inbounds = [
          {
            tag = "trojan-in";
            type = "trojan";

            listen = "::";
            listen_port = 443;

            users = map (name: {
              inherit name;
              password._secret = config.sops.secrets."trojan/${name}-password".path;
            }) cfg.clients;

            tls = {
              enabled = true;
              server_name = "elxreno.com";
              certificate_path = "${config.security.acme.certs."elxreno.com".directory}/cert.pem";
              key_path = "${config.security.acme.certs."elxreno.com".directory}/key.pem";
            };

            fallback = {
              server = "127.0.0.1";
              server_port = 80;
            };

            multiplex = {
              enabled = true;
              padding = true;
            };

            transport = {
              type = "http";
              path = "/configuration/shared/update_client";
              method = "POST";
            };
          }
          {
            tag = "hy2-in";
            type = "hysteria2";

            listen = "::";
            listen_port = 6443;

            obfs = {
              type = "salamander";
              password._secret = config.sops.secrets."hysteria2/obfs-salamander-password".path;
            };

            users = builtins.map (name: {
              inherit name;
              password._secret = config.sops.secrets."hysteria2/${name}-password".path;
            }) cfg.clients;

            ignore_client_bandwidth = true;
            tls = {
              enabled = true;
              server_name = "elxreno.com";
              certificate_path = "${config.security.acme.certs."elxreno.com".directory}/cert.pem";
              key_path = "${config.security.acme.certs."elxreno.com".directory}/key.pem";
            };
            brutal_debug = true;
          }
        ];
      };
    };

    users.users.sing-box.extraGroups = [ "acme" ];
  };
}
