{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    getExe'
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.llm-gateway;
  services = config.${namespace}.services;
  systemctl = getExe' config.systemd.package "systemctl";

  front = r: "${lib.removeSuffix "-backend.service" r.unit}.service";

  managedUnits = lib.concatMap (r: [
    (front r)
    r.unit
  ]) routes;

  unitRunner = pkgs.writeShellApplication {
    name = "llm-gateway-unit";
    runtimeInputs = [ config.systemd.package ];
    text = ''
      systemctl start --no-ask-password "$1"
      while state=$(systemctl show --property=ActiveState --value "$2") && [[ $state != inactive && $state != failed ]]; do
        sleep 2
      done
    '';
  };

  backends = lib.filterAttrs (_: s: s.enable) { inherit (services) freetoken strata xolotl; };

  routes = lib.concatLists (
    lib.mapAttrsToList (
      backend: s:
      lib.mapAttrsToList (name: i: {
        peer = "${backend}-${name}";
        model = i.servedModelName;
        inherit (i) port ctxSize;
        unit = "${backend}-${name}-backend.service";
        toolSearch = lib.elem backend [
          "strata"
          "xolotl"
        ];
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
    enable = mkEnableOption "a llama-swap gateway that routes local model requests by model id, one backend at a time, swapping only once the loaded one has no request in flight";

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

    loadTimeout = mkOption {
      type = types.ints.positive;
      default = 600;
      description = "Seconds a backend may take to become ready, and to stop before the next one starts.";
    };

    models = mkOption {
      type = types.listOf types.attrs;
      readOnly = true;
      default = lib.optionals cfg.enable routes;
      defaultText = "One entry per routed model: peer, model, port, ctxSize, unit and toolSearch.";
      description = "Models the gateway routes, as local clients see them.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.ports.llm-gateway = cfg.port;

    systemd.services = exclusive;

    security.polkit.extraConfig = ''
      polkit.addRule(function(action, subject) {
        if (action.id == "org.freedesktop.systemd1.manage-units" &&
            subject.user == "llama-swap" &&
            ${builtins.toJSON managedUnits}.indexOf(action.lookup("unit")) >= 0 &&
            ["start", "stop"].indexOf(action.lookup("verb")) >= 0) {
          return polkit.Result.YES;
        }
      });
    '';

    services.llama-swap = {
      enable = true;
      listenAddress = "127.0.0.1";
      inherit (cfg) port;
      settings = {
        healthCheckTimeout = cfg.loadTimeout;
        models = lib.listToAttrs (
          map (
            r:
            lib.nameValuePair r.model {
              cmd = "${lib.getExe unitRunner} ${front r} ${r.unit}";
              cmdStop = "${systemctl} stop --no-ask-password ${r.unit}";
              proxy = "http://127.0.0.1:${toString r.port}";
              checkEndpoint = "/health";
              concurrencyLimit = 64;
              timeouts.responseHeader = cfg.responseHeaderTimeout;
            }
          ) routes
        );
      };
    };
  };
}
