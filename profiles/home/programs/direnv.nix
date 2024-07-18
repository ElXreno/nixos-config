{
  home-manager.users.elxreno.programs = {
    direnv = {
      enable = true;
      nix-direnv = { enable = true; };
    };
    bash.enable = true;
  };
}
