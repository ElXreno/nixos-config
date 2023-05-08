{ config, lib, inputs, ... }:

{
  imports = [
    inputs.harmonia.nixosModules.harmonia
  ];

  nix.settings.allowed-users = [ "harmonia" ];

  sops.secrets."nix_store_cache" = { };
  services.harmonia = {
    enable = true;
    signKeyPath = config.sops.secrets."nix_store_cache".path;
  };

  services.tailscale.permitCertUid = config.services.caddy.user;

  services.caddy = lib.mkIf (config.device == "INFINITY") {
    enable = true;
    virtualHosts."infinity.tail1f457.ts.net:10443" = {
      extraConfig = ''
        handle {
          reverse_proxy http://127.0.0.1:5000
        }
        encode zstd
      '';
    };
  };
}
