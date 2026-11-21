{ inputs, ... }:
{
  imports = [ inputs.git-hooks-nix.flakeModule ];

  perSystem =
    { pkgs, ... }:
    {
      pre-commit.settings = {
        install.marker = "flakeModules/git-hooks.nix";

        hooks = {
          treefmt.enable = true;
          convco = {
            enable = true;
            settings.configPath = toString (
              (pkgs.formats.yaml { }).generate "convco.yaml" {
                stripRegex = "^(fixup|squash|amend)! ";
              }
            );
          };
        };
      };
    };
}
