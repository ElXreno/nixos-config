let
  ens3 = {
    matchConfig.Name = "ens3";
    address = [
      "159.195.56.52/22"
      "2a0a:4cc0:c1:e9be::/64"
    ];
    routes = [
      { Gateway = "159.195.56.1"; }
      { Gateway = "fe80::1"; }
    ];
  };
in
{
  networking.useNetworkd = true;

  systemd.network = {
    enable = true;
    networks."10-ens3" = ens3 // {
      linkConfig.RequiredForOnline = "routable";
    };
  };
}
