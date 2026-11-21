{ inputs, config, ... }:
let
  meta = {
    name = "elxreno-clan";
    domain = "angora-ide.ts.net";
  };
in
{
  _module.args.clanMeta = meta;

  clan = {
    imports = [ ../inventory ];

    inherit meta;

    pkgsForSystem =
      system:
      config.allSystems.${system}.clan.pkgs or (import ../nixpkgs.nix {
        inherit inputs;
        namespace = meta.name;
      } system).pkgs;

    modules.pam-rssh = ../clanServices/pam-rssh;
    modules.kopia = ../clanServices/kopia;
    modules.crowdsec = ../clanServices/crowdsec;
    modules.monitoring = ../clanServices/monitoring;
    modules.sing-box = ../clanServices/sing-box;
    modules.tailscale = ../clanServices/tailscale;

    specialArgs = {
      inherit inputs;
      namespace = meta.name;
    };
  };
}
