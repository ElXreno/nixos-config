{
  namespace,
  lib,
  ...
}:

{
  ${namespace}.system.networking.tunnels.warp = {
    mtu = lib.mkDefault 1280;
    generator = {
      name = "wg-warp-conf";
      file = "wg-quick.conf";
      description = "Cloudflare WARP wg-quick config for the netns.";
    };
  };
}
