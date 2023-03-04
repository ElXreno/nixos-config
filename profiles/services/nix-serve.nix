{ config, inputs, ... }:

{
  imports = [ inputs.nix-serve-ng.nixosModules.default ];

  networking.firewall.allowedTCPPorts = [ 5000 ];

  sops.secrets."nix_store_cache" = { };

  services.nix-serve = {
    enable = true;
    secretKeyFile = config.sops.secrets."nix_store_cache".path;
  };
}
