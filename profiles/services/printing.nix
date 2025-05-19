{ pkgs, ... }:
{
  services = {
    avahi = {
      enable = true;
      nssmdns4 = true;
    };
    printing = {
      enable = true;
      drivers = with pkgs; [ gutenprint ];
    };
  };
}
