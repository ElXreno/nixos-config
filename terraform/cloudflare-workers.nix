{
  removed =
    map
      (from: {
        inherit from;
        lifecycle.destroy = false;
      })
      [
        "cloudflare_workers_script.nixos-cache-worker"
        "cloudflare_workers_script.talos-factory-cf-worker-proxy"
        "cloudflare_workers_custom_domain.nixos-cache-worker"
        "cloudflare_workers_custom_domain.talos-factory-cf-worker-proxy"
      ];
}
