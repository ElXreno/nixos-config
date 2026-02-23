{ inputs, ... }:
let
  meta = {
    name = "elxreno-clan";
    domain = "angora-ide.ts.net";
  };
in
{
  _module.args.clanMeta = meta;

  clan = {
    imports = [ ../clanModules ];

    inherit meta;

    specialArgs = {
      inherit inputs;
      namespace = meta.name;
      virtual = false;
    };
  };
}
