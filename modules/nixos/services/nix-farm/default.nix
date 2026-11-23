{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    concatMapStringsSep
    genAttrs
    getExe'
    mkEnableOption
    mkForce
    mkIf
    mkMerge
    mkOption
    optionalString
    toLower
    types
    ;
  cfg = config.${namespace}.services.nix-farm;
  niks3 = config.${namespace}.services.niks3;
  vars = config.clan.core.vars.generators;

  isHub = cfg.role == "hub";
  hostName = toLower config.networking.hostName;
  hubHost = "${cfg.hub}.${config.clan.core.settings.domain}";
  gated = cfg.acOnly || cfg.pauseFor != [ ];

  balancerPort = 50051;
  daemonPort = 50052;
  tunnelPort = 9443;

  caFile = vars.nix-farm-ca.files."ca.crt".path;
  credential = unit: name: "/run/credentials/${unit}.service/${name}";

  leaf =
    {
      cn,
      san,
      owner ? "root",
    }:
    {
      dependencies = [ "nix-farm-ca" ];
      files."cert.pem".secret = false;
      files."key.pem" = {
        secret = true;
        inherit owner;
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
          -subj "/CN=${cn}" -keyout "$out/key.pem" -out request.pem
        printf 'subjectAltName=%s\nextendedKeyUsage=serverAuth,clientAuth\n' "${san}" > extensions
        openssl x509 -req -in request.pem -days 3650 -extfile extensions \
          -CA "$in/nix-farm-ca/ca.crt" -CAkey "$in/nix-farm-ca/ca.key" \
          -set_serial "0x$(openssl rand -hex 16)" -out "$out/cert.pem"
      '';
    };

  systemctl = getExe' config.systemd.package "systemctl";

  guard = pkgs.writeShellScript "nix-farm-guard" ''
    ${optionalString cfg.acOnly ''
      ${getExe' config.systemd.package "systemd-ac-power"} || exit 1
    ''}
    ${concatMapStringsSep "\n" (unit: ''
      ! ${systemctl} is-active --quiet ${unit}.service || exit 1
    '') cfg.pauseFor}
  '';
in
{
  options.${namespace}.services.nix-farm = {
    enable = mkEnableOption "taking part in the nix-grpc-store build farm of the CI";
    role = mkOption {
      type = types.enum [
        "hub"
        "builder"
      ];
      default = if hostName == cfg.hub then "hub" else "builder";
      defaultText = "hub on the host named in `hub`, builder elsewhere";
      description = ''
        The hub schedules, balances and builds; a builder connects to the hub
        and receives its builds over that connection.
      '';
    };
    hub = mkOption {
      type = types.str;
      default = "bimba";
      description = "Tailnet host name of the hub.";
    };
    acOnly = mkEnableOption "building only on mains power";
    pauseFor = mkOption {
      type = types.listOf types.str;
      default = [ ];
      example = [ "llama-cpp-memory" ];
      description = "Services during whose run this builder takes no builds; starting one stops the builds at once.";
    };
    url = mkOption {
      type = types.str;
      readOnly = true;
      default =
        "grpc://${hubHost}:${toString balancerPort}"
        + "?ca-cert=${caFile}"
        + "&client-cert=${vars.nix-farm-ci.files."cert.pem".path}"
        + "&client-key=${vars.nix-farm-ci.files."key.pem".path}";
      description = "Store URL of the farm for the CI on the hub.";
    };
  };

  config = mkIf cfg.enable (mkMerge [
    {
      assertions = [
        {
          assertion = niks3.enable || niks3.push.enable;
          message = "${namespace}.services.nix-farm needs the niks3 push token: enable niks3 or niks3.push on this host.";
        }
      ];

      clan.core.vars.generators = {
        nix-farm-ca = {
          share = true;
          files."ca.crt".secret = false;
          files."ca.key" = {
            secret = true;
            deploy = false;
          };
          runtimeInputs = [ pkgs.openssl ];
          script = ''
            openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 3650 \
              -subj "/CN=nix-farm" \
              -addext "basicConstraints=critical,CA:TRUE" \
              -addext "keyUsage=critical,keyCertSign,cRLSign" \
              -keyout "$out/ca.key" -out "$out/ca.crt"
          '';
        };
        nix-farm-worker = leaf {
          cn = "worker-${hostName}";
          san = if isHub then "IP:127.0.0.1,DNS:${hostName}" else "DNS:${hostName}";
          owner = "nix-grpc-daemon";
        };
      };

      programs.nix-grpc-store.enable = true;

      services.nix-grpc-daemon = {
        enable = true;
        idleTimeout = null;
        tls = {
          certFile = vars.nix-farm-worker.files."cert.pem".path;
          keyFile = vars.nix-farm-worker.files."key.pem".path;
          clientCaFile = caFile;
        };
        trustedProxies = [ "lb-*" ];
        accessRules =
          map
            (cn: {
              inherit cn;
              role = "trusted";
            })
            [
              "ci-*"
              "lb-*"
              "worker-*"
            ];
        niks3 = {
          inherit (config.services.niks3) package;
          inherit (niks3) url;
          cacheUrl = niks3.url;
          publicKeys = [ niks3.publicKey ];
          tokenFile = credential "nix-grpc-daemon" "niks3-token";
        };
      };

      systemd.services.nix-grpc-daemon.serviceConfig.LoadCredential = [
        "niks3-token:${vars.niks3-api-token.files.token.path}"
      ];
    }

    (mkIf isHub {
      ${namespace}.ports = {
        nix-farm = balancerPort;
        nix-farm-daemon = daemonPort;
        nix-farm-tunnel = tunnelPort;
      };

      clan.core.vars.generators = {
        nix-farm-lb = leaf {
          cn = "lb-${hostName}";
          san = "DNS:${hubHost},DNS:${hostName}";
        };
        nix-farm-ci = leaf {
          cn = "ci-${hostName}";
          san = "DNS:${hostName}";
          owner = "nixbot";
        };
      };

      services.nix-grpc-daemon = {
        listen = "127.0.0.1:${toString daemonPort}";
        advertise = "127.0.0.1:${toString daemonPort}";
      };

      services.nix-grpc-farm-lb = {
        enable = true;
        listen = "[::]:${toString balancerPort}";
        systems = [ config.nixpkgs.hostPlatform.system ] ++ config.boot.binfmt.emulatedSystems;
        scheduler = "127.0.0.1:${toString daemonPort}";
        tunnel.listen = "[::]:${toString tunnelPort}";
        tls = rec {
          certFile = vars.nix-farm-lb.files."cert.pem".path;
          keyFile = credential "envoy" "nix-farm-lb-key";
          clientCaFile = caFile;
          upstream = {
            inherit certFile keyFile;
            inherit caFile;
          };
        };
      };

      systemd.services.envoy.serviceConfig.LoadCredential = [
        "nix-farm-lb-key:${vars.nix-farm-lb.files."key.pem".path}"
      ];
    })

    (mkIf (!isHub) {
      services.nix-grpc-daemon = {
        listen = "127.0.0.1:${toString daemonPort}";
        roles = [ "builder" ];
        scheduler = "${hubHost}:${toString balancerPort}";
        tunnel.balancers = [ "${hubHost}:${toString tunnelPort}" ];
      };
    })

    (mkIf (!isHub && gated) {
      systemd = {
        services = {
          nix-grpc-daemon.serviceConfig = {
            ExecCondition = guard;
            Restart = mkForce "on-failure";
            TimeoutStopSec = mkForce "15s";
          };

          nix-farm-pause = {
            description = "Stop the farm builds of this host";
            serviceConfig = {
              Type = "oneshot";
              ExecStart = "${systemctl} stop --no-block nix-grpc-daemon.service";
            };
          };

          nix-farm-guard = {
            description = "Start or stop the farm builder by power source and running services";
            serviceConfig.Type = "oneshot";
            script = ''
              if ${guard}; then
                ${systemctl} start --no-block nix-grpc-daemon.service
              else
                ${systemctl} stop --no-block nix-grpc-daemon.service
              fi
            '';
          };
        }
        // genAttrs cfg.pauseFor (_: {
          wants = [ "nix-farm-pause.service" ];
        });

        timers.nix-farm-guard = {
          wantedBy = [ "timers.target" ];
          timerConfig = {
            OnBootSec = "2min";
            OnUnitActiveSec = "1min";
          };
        };
      };
    })
  ]);
}
