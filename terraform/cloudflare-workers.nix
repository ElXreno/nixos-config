{
  config,
  lib,
  dns,
  ...
}:
let
  accountId = config.data.cloudflare_zone.this "account.id";

  workers = {
    nixos-cache-worker = {
      hostname = "nixos-cache-proxy.${dns.zone}";
      module = "worker.js";
      compatibilityDate = "2026-01-20";
      traces = true;
      placement.mode = "smart";
    };
    talos-factory-cf-worker-proxy = {
      hostname = "factory.talos.${dns.zone}";
      module = "index.js";
      compatibilityDate = "2026-01-31";
      traces = false;
    };
  };
in
{
  resource.cloudflare_workers_script = lib.mapAttrs (
    name: worker:
    {
      account_id = accountId;
      script_name = name;
      main_module = worker.module;
      content = lib.replaceStrings [ "\${" "%{" ] [ "$\${" "%%{" ] (
        builtins.readFile (./workers + "/${name}.js")
      );
      compatibility_date = worker.compatibilityDate;
      observability = {
        enabled = false;
        head_sampling_rate = 1;
        logs = {
          enabled = true;
          head_sampling_rate = 1;
          invocation_logs = true;
          persist = true;
        };
        traces = {
          enabled = worker.traces;
          head_sampling_rate = 1;
          persist = true;
        };
      };
    }
    // lib.optionalAttrs (worker ? placement) { inherit (worker) placement; }
  ) workers;

  resource.cloudflare_workers_custom_domain = lib.mapAttrs (name: worker: {
    account_id = accountId;
    zone_id = config.data.cloudflare_zone.this "zone_id";
    inherit (worker) hostname;
    service = config.resource.cloudflare_workers_script.${name} "script_name";
  }) workers;
}
