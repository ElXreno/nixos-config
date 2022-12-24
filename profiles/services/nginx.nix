{ config, lib, ... }:

{
  security.acme = {
    acceptTerms = true;
    defaults.email = "elxreno@gmail.com";
    # defaults.server = "https://acme-staging-v02.api.letsencrypt.org/directory";
  };

  networking.firewall.allowedTCPPorts = [
    80
    443
  ];

  services = {
    nginx = {
      enable = true;
      recommendedTlsSettings = true;
      recommendedProxySettings = true;
      recommendedOptimisation = true;
      recommendedGzipSettings = true;
      virtualHosts = {
        "elxreno.me" = lib.mkIf (config.device == "Nixis-Server") {
          forceSSL = true;
          enableACME = true;
          root = "/var/www/elxreno.me";
        };
        "funquiz.elxreno.me" = lib.mkIf (config.device == "Nixis-Server" && config.services.fun-quiz-api.enable) {
          forceSSL = true;
          enableACME = true;
          locations."/" = {
            proxyPass = "http://localhost:${toString config.services.fun-quiz-api.port}";
            extraConfig = "proxy_pass_header Authorization;";
          };
        };
        # "code.elxreno.ninja" = lib.mkIf (config.device == "Noxer-Server" && config.services.gitea.enable) {
        #   addSSL = true;
        #   enableACME = true;
        #   locations."/" = { proxyPass = "http://localhost:${toString config.services.gitea.httpPort}"; };
        # };
        # "api.elxreno.ninja" = lib.mkIf (config.device == "Noxer-Server" && config.services.smart-home-server.enable) {
        #   addSSL = true;
        #   enableACME = true;
        # };
        # "ipfs.elxreno.ninja" = lib.mkIf config.services.ipfs.enable {
        #   addSSL = true;
        #   enableACME = true;
        #   locations."~ '^/(ipfs|ipns|api)(/|$)'".proxyPass = "http://127.0.0.1:8080";
        #   locations."/".return = "301 https://elxreno.ninja/";
        # };
        # "elxreno.ninja" = lib.mkIf (config.device == "Noxer-Server") {
        #   addSSL = true;
        #   enableACME = true;
        #   locations = {
        #     "/downloads" = {
        #       extraConfig = ''
        #         if (!-e $request_filename) {
        #           rewrite ^/(.*)$ https://downloads.elxreno.ninja/$1 permanent;
        #         }
        #       '';
        #     };
        #   };
        # };
        # "downloads.elxreno.ninja" = lib.mkIf (config.device == "Nixis-Server") {
        #   addSSL = true;
        #   enableACME = true;
        #   root = "/var/www/elxreno.ninja";
        #   locations = {
        #     "~ \.php$" = {
        #       extraConfig = ''
        #         fastcgi_pass  unix:${config.services.phpfpm.pools.defpool.socket};
        #         fastcgi_index index.php;
        #       '';
        #     };

        #     "/" = {
        #       return = "301 https://$host/downloads/";
        #     };

        #     "/downloads" = {
        #       index = "/downloads/_h5ai/public/index.php";
        #       extraConfig = ''
        #         if (!-e $request_filename) {
        #           rewrite ^/downloads/(.*)$ https://sourceforge.net/projects/elxreno/files/$1 permanent;
        #         }
        #       '';
        #     };
        #   };
        # };
      };
    };

    # phpfpm.pools.defpool = lib.mkIf (config.device == "Nixis-Server") {
    #   user = "nobody";
    #   settings = {
    #     pm = "dynamic";
    #     "listen.owner" = config.services.nginx.user;
    #     "pm.max_children" = 5;
    #     "pm.start_servers" = 2;
    #     "pm.min_spare_servers" = 1;
    #     "pm.max_spare_servers" = 3;
    #     "pm.max_requests" = 100;
    #   };
    # };
  };
}
