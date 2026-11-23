{
  config,
  namespace,
  lib,
  pkgs,
  ...
}@args:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    escapeShellArgs
    types
    ;
  cfg = config.${namespace}.services.tei;
  tei = import ../_cuda-variant.nix args "text-embeddings-inference";
  idle = import ../_idle-backend.nix args;

  runner =
    name: i:
    idle.runner {
      name = "tei-${name}-run";
      command = lib.concatStringsSep " \\\n  " [
        "${tei}/bin/text-embeddings-router"
        "--model-id ${i.model}"
        "--dtype ${i.dtype}"
        "--hostname 127.0.0.1"
        "--port ${toString i.backendPort}"
        "--prometheus-port ${toString i.prometheusPort}"
        (escapeShellArgs i.extraArgs)
      ];
      healthCheck = ''${pkgs.curl}/bin/curl -sf -m 2 "http://127.0.0.1:${toString i.backendPort}/health" >/dev/null 2>&1'';
      interval = "0.5";
    };

  instanceOpts = types.submodule {
    options = idle.instanceOptions { idleTimeout = "5min"; } // {
      model = mkOption {
        type = types.str;
        description = "HuggingFace model id to serve.";
      };
      prometheusPort = mkOption {
        type = types.port;
        description = "TEI metrics port.";
      };
      dtype = mkOption {
        type = types.str;
        default = "float16";
      };
    };
  };
in
{
  options.${namespace}.services.tei = {
    enable = mkEnableOption "running text-embeddings-inference servers";

    hfHome = mkOption {
      type = types.str;
      default = "/var/lib/tei";
      description = "HuggingFace cache directory for downloaded weights.";
    };

    instances = mkOption {
      type = types.attrsOf instanceOpts;
      default = { };
    };
  };

  config = mkIf cfg.enable (
    lib.mkMerge [
      (idle.units {
        name = "tei";
        label = "TEI";
        stateDir = cfg.hfHome;
        inherit (cfg) instances;
        describe = i: i.model;
        backend = name: i: {
          environment = {
            HF_HOME = cfg.hfHome;
            CUDA_CACHE_PATH = "${cfg.hfHome}/ComputeCache";
          };
          serviceConfig.ExecStart = runner name i;
        };
      })
      {
        ${namespace}.ports = lib.mapAttrs' (
          name: i: lib.nameValuePair "tei-${name}-metrics" i.prometheusPort
        ) cfg.instances;
      }
    ]
  );
}
