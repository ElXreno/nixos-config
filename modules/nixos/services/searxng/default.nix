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
    mkOption
    types
    ;
  cfg = config.${namespace}.services.searxng;
in
{
  options.${namespace}.services.searxng = {
    enable = mkEnableOption "a local SearXNG instance for model web search";

    port = mkOption {
      type = types.port;
      default = 8888;
      description = "Port SearXNG binds on 127.0.0.1.";
    };

    engines = mkOption {
      type = types.listOf types.str;
      default = [
        "duckduckgo"
        "yahoo"
        "bing"
        "mwmbl"
        "wikipedia"
        "github"
        "stackoverflow"
        "superuser"
        "askubuntu"
        "arxiv"
      ];
      description = "Engines kept from SearXNG's defaults and enabled; every other engine is dropped.";
    };
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.searxng = {
      files.env.secret = true;
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        echo "SEARXNG_SECRET=$(openssl rand -hex 32)" > "$out/env"
      '';
    };

    services.searx = {
      enable = true;
      environmentFile = config.clan.core.vars.generators.searxng.files.env.path;
      settings = {
        use_default_settings.engines.keep_only = cfg.engines;
        engines = map (name: {
          inherit name;
          disabled = false;
        }) cfg.engines;
        general.instance_name = "searxng";
        server = {
          bind_address = "127.0.0.1";
          inherit (cfg) port;
          secret_key = "$SEARXNG_SECRET";
          limiter = false;
          public_instance = false;
          image_proxy = false;
        };
        search = {
          safe_search = 0;
          formats = [
            "html"
            "json"
          ];
        };
      };
    };

    systemd.services.searx.serviceConfig = {
      Restart = "on-failure";
      RestartSec = 10;
    };

    ${namespace}.ports.searxng = cfg.port;
  };
}
