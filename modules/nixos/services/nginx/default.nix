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
    mkPackageOption
    mkOption
    mkMerge
    types
    mapAttrs
    ;
  cfg = config.${namespace}.services.nginx;

  commonVirtualHostCfg = {
    enableACME = true;
    forceSSL = true;
    quic = true;
    kTLS = true;

    extraConfig = ''
      # 0-RTT: Enable TLS 1.3 early data
      ssl_early_data on;
      # Enables sending in optimized batch mode using segmentation offloading.
      quic_gso on;
      # Enables the QUIC Address Validation feature. This includes sending a new
      # token in a Retry packet or a NEW_TOKEN frame and validating a token
      # received in the initial packet.
      quic_retry on;

      # Advertise http3, not done by NixOS option http3=true yet
      add_header Alt-Svc 'h3=":443"; ma=86400';

      # Other stuff
      set_real_ip_from 127.0.0.1;
      real_ip_header proxy_protocol;

      add_header X-Frame-Options DENY;
      add_header X-Content-Type-Options nosniff;
      add_header X-XSS-Protection "1; mode=block";
      add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload";
    '';
  };
in
{
  options.${namespace}.services.nginx = {
    enable = mkEnableOption "Whether or not to manage nginx.";
    package = mkPackageOption pkgs "nginxQuic" { };
    virtualHosts = mkOption {
      type = with types; attrs;
      default = { };
    };
    configureFail2ban = mkEnableOption "Whether to configure fail2ban service." // {
      default = true;
    };
  };

  config = mkIf cfg.enable (mkMerge [
    {
      networking.firewall = {
        allowedTCPPorts = [ 443 ];
        allowedUDPPorts = [ 443 ];
      };

      services.nginx = {
        enable = true;

        enableReload = true;
        enableQuicBPF = true;

        inherit (cfg) package;

        recommendedTlsSettings = true;

        recommendedGzipSettings = true;
        recommendedBrotliSettings = true;

        recommendedProxySettings = true;
        proxyTimeout = "86400s";

        recommendedOptimisation = true;

        clientMaxBodySize = "4g";

        defaultListen = [
          {
            addr = "0.0.0.0";
            port = 443;
            ssl = true;
          }
        ];

        virtualHosts = mkMerge [
          {
            "elxreno.com" = commonVirtualHostCfg // {
              serverAliases = [ "www.elxreno.com" ];
              default = true;
              locations."= /" = {
                return = "200 '<html><body>Hello there!</body></html>'";
                extraConfig = ''
                  default_type text/html;
                '';
              };
            };
          }
          (mapAttrs (_virtualHost: virtualHostCfg: virtualHostCfg // commonVirtualHostCfg) cfg.virtualHosts)
        ];
      };

      security.acme = {
        acceptTerms = true;
        defaults.email = "elxreno@gmail.com";
      };
    }
    (mkIf cfg.configureFail2ban {
      services.fail2ban.jails = {
        nginx-general.settings = {
          enabled = true;
          port = "80,443";
          protocol = "tcp,udp";
          filter = "nginx-general";
          logpath = "%(nginx_access_log)s";
          backend = "auto";
        };
      };

      environment.etc."fail2ban/filter.d/nginx-general.conf".text = ''
        [Definition]
        failregex = ^<HOST> - .* "(GET|POST|HEAD)(?! \/(favicon\.ico|.*\/.*\.nar(info)?)) .*" (404|444|403|400) .*$
      '';
    })
  ]);
}
