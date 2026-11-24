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
    ;
  cfg = config.${namespace}.services.llm-gateway;
  services = config.${namespace}.services;

  backends = lib.filterAttrs (_: s: s.enable) { inherit (services) freetoken strata xolotl; };

  routes = lib.concatLists (
    lib.mapAttrsToList (
      backend: s:
      lib.mapAttrsToList (name: i: {
        peer = "${backend}-${name}";
        model = i.servedModelName;
        inherit (i) port ctxSize;
        unit = "${backend}-${name}-backend.service";
      }) s.instances
    ) backends
  );

  exclusive = lib.listToAttrs (
    lib.imap1 (
      n: r:
      let
        earlier = map (e: e.unit) (lib.take n routes);
      in
      lib.nameValuePair (lib.removeSuffix ".service" r.unit) {
        conflicts = earlier;
        after = earlier;
      }
    ) (lib.drop 1 routes)
  );
in
{
  options.${namespace}.services.llm-gateway = {
    enable = mkEnableOption "a llama-swap gateway that routes local model requests by model id, one backend at a time";

    port = mkOption {
      type = types.port;
      default = 8090;
      description = "Port the gateway binds on 127.0.0.1.";
    };

    responseHeaderTimeout = mkOption {
      type = types.ints.unsigned;
      default = 1800;
      description = "Seconds a backend may take to send response headers, covering a cold start and a long prefill.";
    };

    models = mkOption {
      type = types.listOf types.attrs;
      readOnly = true;
      default = lib.optionals cfg.enable routes;
      defaultText = "One entry per routed model: peer, model, port, ctxSize and unit.";
      description = "Models the gateway routes, as local clients see them.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.ports.llm-gateway = cfg.port;

    systemd.services = exclusive;

    services.llama-swap = {
      enable = true;
      listenAddress = "127.0.0.1";
      inherit (cfg) port;
      settings.peers = lib.listToAttrs (
        map (
          r:
          lib.nameValuePair r.peer {
            proxy = "http://127.0.0.1:${toString r.port}";
            models = [ r.model ];
            timeouts.responseHeader = cfg.responseHeaderTimeout;
          }
        ) routes
      );
    };
  };
}
