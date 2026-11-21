{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    concatMap
    concatStringsSep
    isList
    mkEnableOption
    mkIf
    mkOption
    optionalAttrs
    types
    ;
  cfg = config.${namespace}.services.manticore;
  ports = config.${namespace}.ports;

  listen = concatMap (host: [
    "${host}:${toString ports.manticore-mysql}:mysql"
    "${host}:${toString ports.manticore-http}:http"
  ]) cfg.listenAddresses;

  renderValue = v: if lib.isBool v then (if v then "1" else "0") else toString v;

  renderSetting =
    name: value: map (v: "    ${name} = ${renderValue v}") (if isList value then value else [ value ]);

  searchdSettings = {
    inherit listen;
    data_dir = cfg.dataDir;
    pid_file = "/run/manticore/searchd.pid";
    log = "${cfg.dataDir}/log/searchd.log";
    buddy_path = "";
  }
  // optionalAttrs (cfg.timezone != null) { inherit (cfg) timezone; }
  // cfg.settings;

  configFile = pkgs.writeText "manticore.conf" ''
    searchd
    {
    ${concatStringsSep "\n" (
      concatMap (name: renderSetting name searchdSettings.${name}) (lib.attrNames searchdSettings)
    )}
    }
  '';

  checkedConfig =
    pkgs.runCommand "manticore.conf-checked"
      {
        nativeBuildInputs = [ cfg.package ];
      }
      ''
        sed -e "s|${cfg.dataDir}|$TMPDIR/data|g" -e "s|/run/manticore|$TMPDIR/run|g" ${configFile} > test.conf
        mkdir -p $TMPDIR/data/log $TMPDIR/run
        searchd --config test.conf --check
        cp ${configFile} $out
      '';
in
{
  options.${namespace}.services.manticore = {
    enable = mkEnableOption "the Manticore Search server";

    package = mkOption {
      type = types.package;
      default = pkgs.${namespace}.manticoresearch;
      description = "Manticore Search package providing searchd.";
    };

    dataDir = mkOption {
      type = types.str;
      default = "/var/lib/manticore";
      description = "Directory of the real-time mode tables, binlog and log.";
    };

    listenAddresses = mkOption {
      type = types.listOf (types.strMatching "[0-9.]+");
      default = [ "127.0.0.1" ];
      description = "IPv4 addresses the MySQL and HTTP listeners bind to; searchd has no IPv6 listeners.";
    };

    mysqlPort = mkOption {
      type = types.port;
      default = 9306;
      description = "Port of the MySQL protocol listener.";
    };

    httpPort = mkOption {
      type = types.port;
      default = 9308;
      description = "Port of the HTTP listener.";
    };

    timezone = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "Europe/Minsk";
      description = "IANA time zone of the date functions; null uses the local zone of the host.";
    };

    embeddings.enable = mkEnableOption "automatic embeddings for KNN attributes (model_name/from)";

    settings = mkOption {
      type =
        with types;
        attrsOf (oneOf [
          bool
          int
          str
          (listOf (oneOf [
            int
            str
          ]))
        ]);
      default = { };
      example = {
        max_open_files = "max";
        secondary_indexes = 1;
        query_log = "/var/lib/manticore/log/query.log";
      };
      description = "Extra searchd settings rendered into the searchd section; a list becomes repeated keys.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports = {
        manticore-mysql = cfg.mysqlPort;
        manticore-http = cfg.httpPort;
      };
      system.impermanence.directories = [
        {
          directory = cfg.dataDir;
          user = "manticore";
          group = "manticore";
          mode = "0750";
        }
      ];
    };

    users.users.manticore = {
      isSystemUser = true;
      group = "manticore";
      home = cfg.dataDir;
    };
    users.groups.manticore = { };

    environment.systemPackages = [ cfg.package ];

    systemd.tmpfiles.rules = [
      "d ${cfg.dataDir} 0750 manticore manticore -"
      "d ${cfg.dataDir}/log 0750 manticore manticore -"
    ];

    systemd.services.manticore = {
      description = "Manticore Search";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      restartTriggers = [ checkedConfig ];
      environment = optionalAttrs cfg.embeddings.enable {
        LIB_MANTICORE_KNN_EMBEDDINGS = "${cfg.package.embeddings}/share/manticore/modules/lib_manticore_knn_embeddings.so";
      };
      serviceConfig = {
        Type = "notify";
        NotifyAccess = "all";
        ExecStart = "${cfg.package}/bin/searchd --config ${checkedConfig} --nodetach";
        ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
        KillMode = "mixed";
        KillSignal = "SIGTERM";
        Restart = "on-failure";
        TimeoutStartSec = "infinity";
        TimeoutStopSec = "10min";
        User = "manticore";
        Group = "manticore";
        StateDirectory = mkIf (cfg.dataDir == "/var/lib/manticore") "manticore";
        StateDirectoryMode = "0750";
        RuntimeDirectory = "manticore";
        WorkingDirectory = cfg.dataDir;
        LimitNOFILE = 1048576;
        LimitMEMLOCK = "infinity";
        NoNewPrivileges = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectHome = true;
        ProtectSystem = "strict";
        ReadWritePaths = [ cfg.dataDir ];
        ProtectClock = true;
        ProtectControlGroups = true;
        ProtectHostname = true;
        ProtectKernelLogs = true;
        ProtectKernelModules = true;
        ProtectKernelTunables = true;
        ProtectProc = "invisible";
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
          "AF_UNIX"
        ];
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        LockPersonality = true;
        SystemCallArchitectures = "native";
        CapabilityBoundingSet = "";
        UMask = "0027";
      };
    };
  };
}
