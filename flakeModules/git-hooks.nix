{ inputs, ... }:
{
  imports = [ inputs.git-hooks-nix.flakeModule ];

  perSystem.pre-commit.settings.hooks = {
    # Formatting — auto-detects treefmt-nix wrapper
    treefmt.enable = true;
    # Conventional commits
    convco.enable = true;
    # Typo detection
    typos.enable = true;
  };
}
