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

    modules.pam-rssh = ../clanServices/pam-rssh;

    specialArgs = {
      inherit inputs;
      lib' = import ../lib { inherit inputs; };
      namespace = meta.name;
      virtual = false;
    };
  };
}
