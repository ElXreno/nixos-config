{ config, inputs, pkgs, lib, ... }:

{
  imports =
    [
      ./wireguard.nix
      "${inputs.nixpkgs}/nixos/modules/virtualisation/azure-common.nix"
      inputs.fun-quiz-server.nixosModules.x86_64-linux.default
      inputs.self.nixosProfiles.nginx
      inputs.self.nixosRoles.server
    ];

  security.sudo.wheelNeedsPassword = false;

  services.tailscale.enable = true;

  services.postgresql = {
    enable = true;
    package = pkgs.postgresql_15;
    authentication = pkgs.lib.mkOverride 10 ''
      local all all trust
      host all all 127.0.0.1/32 trust
      host all all ::1/128 trust
    '';
  };

  sops.secrets."fun-quiz-connection-string" = { };
  sops.secrets."fun-quiz-jwt-secret" = { };

  services.fun-quiz-api = {
    enable = true;
    databaseConnectionUrlFile = config.sops.secrets."fun-quiz-connection-string".path;
    package = pkgs.fun-quiz-server;
    jwt = {
      issuer = "FunQuizApi";
      secretFile = config.sops.secrets."fun-quiz-jwt-secret".path;
    };
    user = config.users.users.root.name;
    inherit (config.users.users.root) group;
  };

  system.stateVersion = "22.05";
}
