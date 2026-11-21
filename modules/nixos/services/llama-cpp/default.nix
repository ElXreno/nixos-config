{
  config,
  namespace,
  lib,
  pkgs,
  ...
}@args:

let
  inherit (lib)
    concatStringsSep
    escapeShellArg
    escapeShellArgs
    getExe'
    mkEnableOption
    mkIf
    mkOption
    types
    ;
  cfg = config.${namespace}.services.llama-cpp;
  idle = import ../_idle-backend.nix args;

  runner =
    name: i:
    idle.runner {
      name = "llama-cpp-${name}-run";
      command = concatStringsSep " \\\n  " [
        (getExe' cfg.package "llama-server")
        "--model ${escapeShellArg i.model}"
        "--host 127.0.0.1"
        "--port ${toString i.backendPort}"
        (escapeShellArgs i.extraArgs)
      ];
      healthCheck = ''${pkgs.curl}/bin/curl -sf -m 2 "http://127.0.0.1:${toString i.backendPort}/health" >/dev/null 2>&1'';
      interval = "0.5";
    };

  instanceOpts = types.submodule {
    options = idle.instanceOptions { idleTimeout = "5min"; } // {
      model = mkOption {
        type = types.str;
        description = "Path to the GGUF file on the host.";
      };
    };
  };
in
{
  options.${namespace}.services.llama-cpp = {
    enable = mkEnableOption "running llama.cpp servers";

    package = mkOption {
      type = types.package;
      default = pkgs.llama-cpp;
      description = "llama.cpp package providing llama-server.";
    };

    stateDir = mkOption {
      type = types.str;
      default = "/var/lib/llama-cpp";
      description = "State directory of the llama.cpp servers.";
    };

    instances = mkOption {
      type = types.attrsOf instanceOpts;
      default = { };
    };
  };

  config = mkIf cfg.enable (
    idle.units {
      name = "llama-cpp";
      label = "llama.cpp";
      inherit (cfg) stateDir instances;
      describe = i: baseNameOf i.model;
      backend = name: i: {
        environment.HOME = cfg.stateDir;
        serviceConfig.ExecStart = runner name i;
      };
    }
  );
}
