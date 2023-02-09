{ config, inputs, pkgs, ... }:

{
  imports = [ inputs.nix-serve-ng.nixosModules.default ];

  networking.firewall.allowedTCPPorts = [ 5000 ];

  sops.secrets."nix_store_cache" = { };

  services.nix-serve = {
    enable = true;
    # https://github.com/NixOS/nix/issues/7704
    package = pkgs.nix-serve.override {
      nix = pkgs.nixVersions.nix_2_12;
    };
    secretKeyFile = config.sops.secrets."nix_store_cache".path;
  };
}
