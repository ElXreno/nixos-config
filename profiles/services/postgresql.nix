{ config, lib, ... }: {
  services.postgresql = {
    enable = true;
    settings = { listen_addresses = lib.mkForce "*"; };
    authentication = lib.optionalString config.services.tailscale.enable ''
      host all all 100.64.0.0/10 trust
    '';
  };
}
