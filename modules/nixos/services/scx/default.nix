{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.scx;

  sortedLayers = lib.pipe cfg.layered.layers [
    lib.attrValues
    (builtins.sort (a: b: a.priority < b.priority))
    (map (l: removeAttrs l [ "priority" ]))
  ];

  layeredConfigFile = pkgs.writeText "scx-layered.json" (builtins.toJSON sortedLayers);

  isLayered = cfg.scheduler == "scx_layered";
in
{
  options.${namespace}.services.scx = {
    enable = mkEnableOption "Whether or not to manage scx.";
    scheduler = mkOption {
      type = with types; str;
      default = "scx_lavd";
    };
    schedulerExtraArgs = mkOption {
      type = with types; listOf str;
      default = [ ];
    };
    layered.layers = mkOption {
      type = with types; attrsOf attrs;
      default = { };
      description = ''
        Layer specs for `scx_layered`. Sorted ascending by `priority`
        (lower = earlier in array = matched first). The `priority` field is
        stripped before JSON serialization. Has no effect when scheduler is
        not `scx_layered`.
      '';
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.services.scx = {
      schedulerExtraArgs = mkIf isLayered [ "file:${layeredConfigFile}" ];
      layered.layers.catchall = mkIf isLayered (
        lib.mkDefault {
          name = "default";
          priority = 9999;
          matches = [ [ ] ];
          kind.Open = { };
        }
      );
    };

    services.scx = {
      enable = true;
      inherit (cfg) scheduler;
      extraArgs = cfg.schedulerExtraArgs;
    };
  };
}
