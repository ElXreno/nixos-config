{
  inventory.instances.tailscale = {
    module.name = "tailscale";
    module.input = "self";
    roles.node.tags = [ "all" ];
    roles.exit-node.tags = [ "server" ];
  };
}
