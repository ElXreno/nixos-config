{
  inventory.instances.crowdsec = {
    module.name = "crowdsec";
    module.input = "self";
    roles.server.machines.BIMBA = { };
    roles.client.tags = [
      "laptop"
      "server"
    ];
  };
}
