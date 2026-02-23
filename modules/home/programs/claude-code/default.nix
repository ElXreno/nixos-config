{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.claude-code;
in
{
  options.${namespace}.programs.claude-code = {
    enable = mkEnableOption "Whether or not to manage Claude Code.";
  };

  config = mkIf cfg.enable {
    programs.claude-code.enable = true;
  };
}
