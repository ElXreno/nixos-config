{
  inventory.instances.pam-rssh = {
    module.name = "pam-rssh";
    module.input = "self";
    roles.server.tags = [ "server" ];
    roles.client.tags = [ "laptop" ];
  };
}
