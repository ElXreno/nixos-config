{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mapAttrs'
    mkOption
    nameValuePair
    types
    ;
in
{
  instanceOptions =
    { idleTimeout }:
    {
      port = mkOption {
        type = types.port;
        description = "Socket-activated front port clients connect to.";
      };
      backendPort = mkOption {
        type = types.port;
        description = "Internal port the backend binds on 127.0.0.1.";
      };
      idleTimeout = mkOption {
        type = types.str;
        default = idleTimeout;
        description = "Stop the backend after this long idle, freeing the GPU.";
      };
      extraArgs = mkOption {
        type = types.listOf types.str;
        default = [ ];
      };
    };

  runner =
    {
      name,
      command,
      healthCheck,
      interval,
    }:
    pkgs.writeShellScript name ''
      ${command} &
      pid=$!
      trap 'kill -TERM "$pid" 2>/dev/null; wait "$pid"; exit 0' TERM INT
      until ${healthCheck}; do
        kill -0 "$pid" 2>/dev/null || exit 1
        sleep ${interval}
      done
      ${config.systemd.package}/bin/systemd-notify --ready
      wait "$pid"
    '';

  units =
    {
      name,
      label,
      stateDir,
      instances,
      describe,
      extraAddressFamilies ? [ ],
      backend,
    }:
    let
      hardening = {
        User = name;
        Group = name;
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        ProtectSystem = "strict";
        ReadWritePaths = [ stateDir ];
        ProtectControlGroups = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
          "AF_UNIX"
        ]
        ++ extraAddressFamilies;
        RestrictRealtime = true;
        LockPersonality = true;
      };
    in
    {
      users.users.${name} = {
        isSystemUser = true;
        group = name;
      };
      users.groups.${name} = { };

      systemd.tmpfiles.rules = [ "d ${stateDir} 0750 ${name} ${name} -" ];

      ${namespace} = {
        system.impermanence.directories = [
          {
            directory = stateDir;
            user = name;
            group = name;
            mode = "0750";
          }
        ];

        ports = lib.concatMapAttrs (instance: i: {
          "${name}-${instance}" = i.port;
          "${name}-${instance}-backend" = i.backendPort;
        }) instances;
      };

      # 0.0.0.0 so podman containers reach it via host.containers.internal; the firewall keeps it closed
      systemd.sockets = mapAttrs' (
        instance: i:
        nameValuePair "${name}-${instance}" {
          wantedBy = [ "sockets.target" ];
          listenStreams = [ "0.0.0.0:${toString i.port}" ];
        }
      ) instances;

      systemd.services = lib.mkMerge [
        (mapAttrs' (
          instance: i:
          nameValuePair "${name}-${instance}" {
            description = "${label} socket proxy (${describe i})";
            requires = [ "${name}-${instance}-backend.service" ];
            after = [ "${name}-${instance}-backend.service" ];
            serviceConfig = hardening // {
              ExecStart = "${config.systemd.package}/lib/systemd/systemd-socket-proxyd --exit-idle-time=${i.idleTimeout} 127.0.0.1:${toString i.backendPort}";
            };
          }
        ) instances)

        (mapAttrs' (
          instance: i:
          nameValuePair "${name}-${instance}-backend" (
            lib.recursiveUpdate {
              description = "${label} backend (${describe i})";
              bindsTo = [ "${name}-${instance}.service" ];
              serviceConfig = hardening // {
                Type = "notify";
                NotifyAccess = "all";
                Restart = "on-failure";
              };
            } (backend instance i)
          )
        ) instances)
      ];
    };
}
