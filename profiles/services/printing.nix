{ pkgs, ... }:
{
  services = {
    avahi = {
      enable = true;
      nssmdns = true;
    };
    printing = {
      enable = true;
      drivers = with pkgs; [ gutenprint /* cnijfilter2 */ ];
    };
  };
}
