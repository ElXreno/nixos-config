{ config, inputs, pkgs, lib, ... }:

{
  imports =
    [
      "${inputs.nixpkgs}/nixos/modules/virtualisation/digital-ocean-config.nix"
      inputs.self.nixosRoles.server
      inputs.self.nixosProfiles.boinc
      # inputs.self.nixosProfiles.nginx
      ./wireguard.nix
    ];

  sops.secrets.coturn = { };

  # services.coturn = rec {
  #   enable = true;
  #   lt-cred-mech = true;
  #   use-auth-secret = true;
  #   static-auth-secret-file = sops.secrets.coturn.path;
  #   realm = "turn.elxreno.ninja";
  #   no-tcp-relay = true;
  #   extraConfig = "
  #     cipher-list=\"HIGH\"
  #     no-loopback-peers
  #     no-multicast-peers
  #   ";
  #   secure-stun = true;
  #   cert = "${config.security.acme.certs.${realm}.directory}/full.pem";
  #   pkey = "${config.security.acme.certs.${realm}.directory}/key.pem";
  #   min-port = 49152;
  #   max-port = 49999;
  # };

  # networking.firewall =
  #   let
  #     range = with config.services.coturn; [{
  #       from = min-port;
  #       to = max-port;
  #     }];
  #   in
  #   {
  #     allowedTCPPorts = [ 3478 3479 5349 5350 ];
  #     allowedTCPPortRanges = range;
  #     allowedUDPPorts = [ 5349 5350 ];
  #     allowedUDPPortRanges = range;
  #   };

  # services.nginx = {
  #   enable = true;
  #   virtualHosts = {
  #     "${config.services.coturn.realm}" = {
  #       forceSSL = true;
  #       enableACME = true;
  #     };
  #   };
  # };

  # users.groups.nginx.members = [ "turnserver" ];

  # security.acme.certs.${config.services.coturn.realm} = {
  #   postRun = "systemctl reload nginx.service; systemctl restart coturn.service";
  # };

  system.stateVersion = "22.05";
}
