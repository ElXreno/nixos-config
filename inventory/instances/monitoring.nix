{
  inventory.instances.monitoring = {
    module.name = "monitoring";
    module.input = "self";
    roles.server.machines.BIMBA = { };
    roles.client.tags = [ "all" ];
  };
}
