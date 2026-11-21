{
  inventory.instances.nixosModules = {
    module.name = "importer";
    roles.default.tags = [ "all" ];
    roles.default.extraModules = [ ../../modules ];
  };
}
