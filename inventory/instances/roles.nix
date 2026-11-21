{ lib, ... }:
let
  roles = [
    "laptop"
    "phone"
    "server"
    "tablet"
    "workstation"
  ];
in
{
  inventory.instances = lib.genAttrs' roles (
    role:
    lib.nameValuePair "role-${role}" {
      module.name = "importer";
      roles.default = {
        tags = [ role ];
        extraModules = [ ({ namespace, ... }: { ${namespace}.roles.${role}.enable = true; }) ];
      };
    }
  );
}
