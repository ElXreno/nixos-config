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
    (lib.mapAttrsToList (name: value: value // { inherit name; }))
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
        Layer specs for `scx_layered`. The layer `name` is taken from the
        attribute name. Sorted ascending by `priority` (lower = earlier in
        array = matched first); `priority` is stripped before JSON
        serialization. Has no effect when scheduler is not `scx_layered`.
      '';
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.services.scx = {
      schedulerExtraArgs = mkIf isLayered [
        "file:${layeredConfigFile}"
        "--percpu-kthread-preempt-all"
      ];
      layered.layers = mkIf isLayered {
        system = lib.mkDefault {
          priority = 4400;
          matches = [
            [ { PcommPrefix = "dbus-broker"; } ]
            [ { PcommPrefix = "systemd"; } ]
          ];
          kind.Open = {
            weight = 1000;
            preempt = true;
            preempt_first = true;
          };
        };
        kthreads = lib.mkDefault {
          priority = 4500;
          matches = [ [ { IsKthread = true; } ] ];
          kind.Open = {
            weight = 5000;
            preempt = true;
            preempt_first = true;
          };
        };
        background-build = lib.mkDefault {
          priority = 1;
          matches = [
            [ { CgroupRegex = ".+nix-daemon.+"; } ]
            [ { CommPrefix = "gcc"; } ]
            [ { CommPrefix = "cc1"; } ]
            [ { CommPrefix = "rustc"; } ]
            [ { CommPrefix = "ld.lld"; } ]
            [ { CommPrefix = "clang"; } ]
            [ { CommPrefix = "cargo"; } ]
          ];
          kind.Open = {
            weight = 1;
          };
        };
        default = lib.mkDefault {
          priority = 9999;
          matches = [ [ ] ];
          kind.Open = {
            weight = 10;
            slice_us = 10000;
          };
        };
      };
    };

    services.scx = {
      enable = true;
      inherit (cfg) scheduler;
      extraArgs = cfg.schedulerExtraArgs;
    };
  };
}
