{ pkgs, lib, ... }:
{
  services.postgresql = {
    enable = true;
    package = pkgs.postgresql_17;
    authentication = lib.mkForce ''
      local all all trust
    '';
  };
}
