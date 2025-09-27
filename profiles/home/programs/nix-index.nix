{ inputs, ... }:
{
  imports = [ inputs.self.nixosProfiles.programs.command-not-found ];
  home-manager.users.elxreno = {
    imports = [ inputs.nix-index-database.homeModules.nix-index ];

    programs = {
      nix-index = {
        enable = true;
        enableBashIntegration = true;
        enableFishIntegration = true;
      };
      nix-index-database.comma.enable = true;
    };
  };
}
