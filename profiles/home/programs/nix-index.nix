{ inputs, ... }:
{
  imports = [ inputs.self.nixosProfiles.command-not-found ];
  home-manager.users.elxreno.programs.nix-index = {
    enable = true;
    enableBashIntegration = true;
    enableFishIntegration = true;
  };
}
