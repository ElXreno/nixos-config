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
    modules.sing-box = ../clanServices/sing-box;
    modules.tailscale = ../clanServices/tailscale;

    specialArgs = {
      inherit inputs;
      namespace = meta.name;
      virtual = false;
    };
  };
}
