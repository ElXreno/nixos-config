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
    modules.crowdsec = ../clanServices/crowdsec;
    modules.tailscale = ../clanServices/tailscale;

    vars.settings.age.postQuantum = true;

    specialArgs = {
      inherit inputs;
      lib' = import ../lib { inherit inputs; };
      namespace = meta.name;
      virtual = false;
    };
  };
}
