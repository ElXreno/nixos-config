{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    mkEnableOption
    mkIf
    mkOption
    types
    ;
  cfg = config.${namespace}.services.nix-farm;
  niks3 = config.${namespace}.services.niks3;
  ports = config.${namespace}.ports;
  vars = config.clan.core.vars.generators;
in
{
  options.${namespace}.services.nix-farm = {
    enable = mkEnableOption "the nix-grpc-store daemon that builds for the CI on this host";
    url = mkOption {
      type = types.str;
      readOnly = true;
      default = "grpc://127.0.0.1:${toString ports.nix-farm}?insecure=1";
      description = "Store URL of the daemon for clients on this host.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.ports.nix-farm = 50051;

    programs.nix-grpc-store.enable = true;

    services.nix-grpc-daemon = {
      enable = true;
      listen = "127.0.0.1:${toString ports.nix-farm}";
      niks3 = {
        inherit (config.services.niks3) package;
        inherit (niks3) url;
        cacheUrl = niks3.url;
        publicKeys = [ niks3.publicKey ];
        tokenFile = "/run/credentials/nix-grpc-daemon.service/niks3-token";
      };
    };

    systemd.services.nix-grpc-daemon.serviceConfig.LoadCredential = [
      "niks3-token:${vars.niks3-api-token.files.token.path}"
    ];
  };
}
