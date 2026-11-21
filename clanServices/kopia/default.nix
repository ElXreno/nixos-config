let
  sourceOptions = import ./source-options.nix;

  commonHomeIgnore = [
    "/.android"
    "/.aspnet"
    "/.cache"
    "/.cargo"
    "/.compose-cache"
    "/.config/**/*Cache*"
    "/.config/syncthing/index*"
    "/.config/zed"
    "/.continue"
    "/.electron"
    "/.electron-gyp"
    "/.flatpak-builder"
    "/.gitkraken"
    "/.go"
    "/.gradle"
    "/.ipfs"
    "/.java"
    "/.javacpp"
    "/.kde"
    "/.kube/cache"
    "/.lighthouse"
    "/.local"
    "/.mozilla"
    "/.nethermind"
    "/.nix-defexpr"
    "/.nix-profile"
    "/.npm"
    "/.nuget"
    "/.nv/ComputeCache"
    "/.ollama"
    "/.rustup"
    "/.steam"
    "/.stellarium"
    "/.var"
  ];

  userDefaults = {
    sources = { };
    extraIgnore = [ ];
    home = {
      enable = true;
      path = null;
    };
    compression = "zstd-better-compression";
    retention = {
      latest = 3;
      hourly = 0;
      daily = 7;
      weekly = 4;
      monthly = 6;
      annual = 1;
    };
    schedule = "04:00";
    verify = {
      enable = true;
      percent = 1;
    };
    globalPolicy = { };
  };
in
{
  _class = "clan.service";
  manifest.name = "kopia";
  manifest.description = "kopia backups of local paths into a per-user S3 repository";
  manifest.categories = [ "System" ];
  manifest.readme = ''
    # kopia

    Every home-manager user of a machine is backed up by default into an
    own kopia repository in the shared bucket under `<user>-<host>/`, with
    own schedule, retention, policies and verification. Units run as root
    and connect under the `<user>@<host>` identity, which also owns
    maintenance.

    The home directory is taken from home-manager and snapshotted with
    `defaultIgnore` plus the user's `extraIgnore`. Declaring a user in
    `users` only overrides those defaults and adds further `sources`.

    Machine-wide state belongs to the service module that owns it:
    a module writes `<namespace>.services.kopia.backup = [ { path = ...; } ]`
    the same way it declares its impermanence directories, and every such
    path lands in the repository of `systemUser` (`system` by default),
    which appears by itself as soon as anything is declared.

    Roles:
    - `default`: per user a `kopia-<instance>-<user>-backup` timer that
      imports the declared policies with `--delete-other-policies`,
      snapshots the user's sources one by one and then downloads and
      verifies a random sample of file data. `kopia-<instance>-<user>`
      wraps the CLI against that repository.

    The repository is created on the first run and only then: the unit
    records a marker in its `StateDirectory` afterwards, so a later
    failure to connect (a renamed prefix, a rotated password) fails
    loudly instead of quietly starting an empty repository beside the
    real one. `kopia-<instance>-<user> init` creates one by hand.

    Vars: `kopia-<instance>-storage` is shared and prompts once for the
    endpoint and the region, `kopia-<instance>` prompts per machine for
    its own S3 key pair. The repository password of each user is generated,
    not asked for: `kopia-<instance>-<user>` holds 48 random base64
    characters. Endpoint and region are read at runtime, so they stay out
    of the store and out of the repository.
  '';

  roles.default = {
    description = "Backs up each user's paths into that user's kopia repository on S3.";
    interface =
      { lib, ... }:
      let
        inherit (lib) mkEnableOption mkOption types;
        keep =
          default:
          mkOption {
            type = types.ints.unsigned;
            inherit default;
          };
        sourceModule = types.submodule { options = sourceOptions lib; };
        userModule = types.submodule {
          options = {
            home = {
              enable = mkEnableOption "snapshotting this user's home directory" // {
                default = userDefaults.home.enable;
              };
              path = mkOption {
                type = types.nullOr types.str;
                default = userDefaults.home.path;
                description = "Home directory, taken from home-manager when null.";
              };
            };

            extraIgnore = mkOption {
              type = types.listOf types.str;
              default = userDefaults.extraIgnore;
              description = "Ignore rules of the home source on top of `defaultIgnore`.";
            };

            sources = mkOption {
              type = types.attrsOf sourceModule;
              default = userDefaults.sources;
              description = "Further paths to back up besides the home directory.";
            };

            compression = mkOption {
              type = types.str;
              default = userDefaults.compression;
              description = "Compressor for every source unless overridden.";
            };

            retention = {
              latest = keep userDefaults.retention.latest;
              hourly = keep userDefaults.retention.hourly;
              daily = keep userDefaults.retention.daily;
              weekly = keep userDefaults.retention.weekly;
              monthly = keep userDefaults.retention.monthly;
              annual = keep userDefaults.retention.annual;
            };

            schedule = mkOption {
              type = types.str;
              default = userDefaults.schedule;
              description = "OnCalendar expression of the backup timer.";
            };

            verify = {
              enable = mkEnableOption "verifying a sample of file data after each backup" // {
                default = userDefaults.verify.enable;
              };
              percent = mkOption {
                type = types.numbers.between 0 100;
                default = userDefaults.verify.percent;
                description = "Share of files downloaded and verified per run.";
              };
            };

            globalPolicy = mkOption {
              type = types.attrsOf types.anything;
              default = userDefaults.globalPolicy;
              description = "Extra fields merged into the repository's `(global)` policy.";
            };
          };
        };
      in
      {
        options = {
          s3 = {
            bucket = mkOption {
              type = types.str;
              description = "Bucket holding the repositories.";
            };
          };

          autoUsers = mkEnableOption "backing up every home-manager user of the machine" // {
            default = true;
          };

          systemUser = mkOption {
            type = types.str;
            default = "system";
            description = "Identity owning the machine-wide paths that service modules declare.";
          };

          defaultIgnore = mkOption {
            type = types.listOf types.str;
            default = commonHomeIgnore;
            description = "Ignore rules applied to every user's home directory.";
          };

          users = mkOption {
            type = types.attrsOf userModule;
            default = { };
            description = "Per-user overrides, and users beyond the home-manager ones.";
          };
        };
      };

    perInstance =
      { instanceName, settings, ... }:
      {
        nixosModule =
          {
            config,
            lib,
            namespace,
            pkgs,
            ...
          }:
          let
            inherit (lib)
              any
              attrNames
              attrValues
              concatMapStringsSep
              concatStringsSep
              escapeShellArg
              filter
              getExe
              head
              lessThan
              listToAttrs
              mapAttrsToList
              mergeAttrsList
              mkDefault
              nameValuePair
              optional
              optionalAttrs
              optionalString
              optionals
              recursiveUpdate
              removeSuffix
              sort
              splitString
              toLower
              unique
              ;

            host = toLower (head (splitString "." config.networking.hostName));
            keys = config.clan.core.vars.generators."kopia-${instanceName}";
            storage = config.clan.core.vars.generators."kopia-${instanceName}-storage";
            normalize = p: if p == "/" then p else removeSuffix "/" p;

            hmUsers = config.home-manager.users or { };
            serviceSources = config.${namespace}.services.kopia.backup;
            userNames = sort lessThan (
              unique (
                (optionals settings.autoUsers (attrNames hmUsers))
                ++ (optional (serviceSources != [ ]) settings.systemUser)
                ++ attrNames settings.users
              )
            );
            userCfg = name: settings.users.${name} or userDefaults;
            homePath =
              name:
              let
                cfg = userCfg name;
              in
              if cfg.home.path != null then
                cfg.home.path
              else if hmUsers ? ${name} then
                hmUsers.${name}.home.homeDirectory
              else
                null;
            sourcesOf =
              name:
              let
                cfg = userCfg name;
              in
              sort (a: b: a.path < b.path) (
                optional (cfg.home.enable && homePath name != null) {
                  path = homePath name;
                  command = null;
                  fileName = null;
                  preCommand = "";
                  postCommand = "";
                  ignore = settings.defaultIgnore ++ cfg.extraIgnore;
                  compression = null;
                  policy = { };
                }
                ++ optionals (name == settings.systemUser) serviceSources
                ++ attrValues cfg.sources
              );

            defaultGlobalPolicy = {
              files.ignoreDotFiles = [ ".kopiaignore" ];
              metadataCompression.compressorName = "zstd-fastest";
              errorHandling = {
                ignoreFileErrors = false;
                ignoreDirectoryErrors = false;
                ignoreUnknownTypes = true;
              };
              scheduling.runMissed = true;
              logging = {
                directories = {
                  snapshotted = 5;
                  ignored = 5;
                };
                entries = {
                  snapshotted = 0;
                  ignored = 5;
                  cacheHit = 0;
                  cacheMiss = 0;
                };
              };
              osSnapshots.volumeShadowCopy.enable = 0;
              upload = {
                maxParallelSnapshots = 1;
                parallelUploadAboveSize = 2147483648;
              };
            };

            sourcePolicy =
              s:
              recursiveUpdate (
                {
                  files.ignore = s.ignore;
                }
                // optionalAttrs (s.compression != null) {
                  compression.compressorName = s.compression;
                }
                // optionalAttrs (s.command != null) {
                  scheduling.manual = true;
                  splitter.algorithm = "DYNAMIC-256K-BUZHASH";
                }
              ) s.policy;

            mkUser =
              name: cfg:
              let
                unit = "kopia-${instanceName}-${name}";
                identity = "${name}@${host}";
                prefix = "${host}/${name}/";
                cacheDir = "/var/cache/kopia/${instanceName}/${name}";
                logDir = "/var/log/kopia/${instanceName}/${name}";
                lockFile = "${cacheDir}/.lock";
                generator = config.clan.core.vars.generators.${unit};
                sources = sourcesOf name;
                r = cfg.retention;

                policies = {
                  "(global)" = recursiveUpdate (
                    defaultGlobalPolicy
                    // {
                      retention = {
                        keepLatest = r.latest;
                        keepHourly = r.hourly;
                        keepDaily = r.daily;
                        keepWeekly = r.weekly;
                        keepMonthly = r.monthly;
                        keepAnnual = r.annual;
                        ignoreIdenticalSnapshots = false;
                      };
                      compression.compressorName = cfg.compression;
                    }
                  ) cfg.globalPolicy;
                }
                // listToAttrs (map (s: nameValuePair "${identity}:${normalize s.path}" (sourcePolicy s)) sources);
                policyFile = pkgs.writeText "${unit}-policies.json" (builtins.toJSON policies);

                repoFlags = concatStringsSep " " [
                  "--bucket=${escapeShellArg settings.s3.bucket}"
                  ''--endpoint="$KOPIA_ENDPOINT"''
                  ''--region="$KOPIA_REGION"''
                  "--prefix=${escapeShellArg prefix}"
                  "--override-username=${escapeShellArg name}"
                  "--override-hostname=${escapeShellArg host}"
                  "--cache-directory=${cacheDir}/cache"
                  "--no-persist-credentials"
                  "--no-check-for-updates"
                ];

                functions = ''
                  kopia_env() {
                    : "''${KOPIA_PASSWORD_FILE:=$CREDENTIALS_DIRECTORY/password}"
                    : "''${KOPIA_ACCESS_KEY_FILE:=$CREDENTIALS_DIRECTORY/access-key}"
                    : "''${KOPIA_SECRET_KEY_FILE:=$CREDENTIALS_DIRECTORY/secret-access-key}"
                    : "''${KOPIA_ENDPOINT_FILE:=$CREDENTIALS_DIRECTORY/endpoint}"
                    : "''${KOPIA_REGION_FILE:=$CREDENTIALS_DIRECTORY/region}"
                    export HOME="$KOPIA_HOME"
                    export KOPIA_CONFIG_PATH="$KOPIA_HOME/repository.config"
                    export KOPIA_LOG_DIR=${logDir}
                    KOPIA_PASSWORD="$(< "$KOPIA_PASSWORD_FILE")"
                    AWS_ACCESS_KEY_ID="$(< "$KOPIA_ACCESS_KEY_FILE")"
                    AWS_SECRET_ACCESS_KEY="$(< "$KOPIA_SECRET_KEY_FILE")"
                    KOPIA_ENDPOINT="$(< "$KOPIA_ENDPOINT_FILE")"
                    KOPIA_REGION="$(< "$KOPIA_REGION_FILE")"
                    export KOPIA_PASSWORD AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY
                  }
                  kopia_connect() {
                    kopia_env
                    kopia repository connect s3 ${repoFlags}
                  }
                '';

                initFunction = ''
                  kopia_init() {
                    kopia_env
                    kopia repository create s3 ${repoFlags}
                  }
                '';

                backupScript = pkgs.writeShellApplication {
                  name = "${unit}-backup";
                  runtimeInputs = [
                    pkgs.kopia
                    pkgs.jq
                    pkgs.util-linux
                  ];
                  text = ''
                    exec 9>${lockFile}
                    flock -x 9
                    KOPIA_HOME="$RUNTIME_DIRECTORY"
                    ${functions}
                    ${initFunction}
                    if ! kopia_connect; then
                      if [ -e "$STATE_DIRECTORY/initialized" ]; then
                        echo "repository ${prefix} was initialized before but is unreachable now; refusing to create a new one" >&2
                        exit 1
                      fi
                      kopia_init
                      touch "$STATE_DIRECTORY/initialized"
                      kopia_connect
                    fi
                    if [ "$(kopia maintenance info --json | jq -r .owner)" != ${escapeShellArg identity} ]; then
                      kopia maintenance set --owner=${escapeShellArg identity}
                    fi
                    kopia policy import --from-file=${policyFile} --delete-other-policies
                    status=0
                    ${
                      optionalString (any (s: s.postCommand != "") sources) ''
                        # shellcheck disable=SC2329 # runs from the EXIT trap
                        unquiesce() { :; }
                        trap unquiesce EXIT
                      ''
                    }${
                      concatMapStringsSep "\n" (
                        s:
                        let
                          snapshot =
                            if s.command == null then
                              "kopia snapshot create ${escapeShellArg s.path} || status=1"
                            else
                              "${s.command} | kopia snapshot create ${escapeShellArg s.path} --stdin-file=${
                                escapeShellArg (if s.fileName != null then s.fileName else baseNameOf s.path)
                              } || status=1";
                        in
                        optionalString (s.postCommand != "") ''
                          # shellcheck disable=SC2329 # runs from the EXIT trap
                          unquiesce() {
                          ${s.postCommand}
                          }
                        ''
                        + optionalString (s.preCommand != "") ''
                          {
                          ${s.preCommand}
                          } || status=1
                        ''
                        + snapshot
                        + optionalString (s.postCommand != "") ''

                          unquiesce || status=1
                          # shellcheck disable=SC2329 # runs from the EXIT trap
                          unquiesce() { :; }''
                      ) sources
                    }
                    ${optionalString cfg.verify.enable "kopia snapshot verify --verify-files-percent=${toString cfg.verify.percent} || status=1"}
                    exit "$status"
                  '';
                };

                wrapper = pkgs.writeShellApplication {
                  name = unit;
                  runtimeInputs = [
                    pkgs.kopia
                    pkgs.coreutils
                  ];
                  text = ''
                    KOPIA_HOME="$(mktemp -d)"
                    trap 'rm -rf "$KOPIA_HOME"' EXIT
                    export KOPIA_PASSWORD_FILE=${generator.files.password.path}
                    export KOPIA_ACCESS_KEY_FILE=${keys.files."access-key".path}
                    export KOPIA_SECRET_KEY_FILE=${keys.files."secret-access-key".path}
                    export KOPIA_ENDPOINT_FILE=${storage.files.endpoint.path}
                    export KOPIA_REGION_FILE=${storage.files.region.path}
                    ${functions}
                    ${initFunction}
                    if [ "''${1:-}" = init ]; then
                      kopia_init
                      exit
                    fi
                    kopia_connect >/dev/null
                    kopia "$@"
                  '';
                };

                mkService = kind: script: extraUnitConfig: {
                  description = "kopia ${kind} (${instanceName}, ${name})";
                  wants = [ "network-online.target" ];
                  after = [ "network-online.target" ];
                  restartIfChanged = false;
                  stopIfChanged = false;
                  unitConfig = extraUnitConfig;
                  serviceConfig = {
                    Type = "oneshot";
                    ExecStart = getExe script;
                    RuntimeDirectory = "${unit}-${kind}";
                    RuntimeDirectoryMode = "0700";
                    CacheDirectory = "kopia/${instanceName}/${name}";
                    CacheDirectoryMode = "0700";
                    StateDirectory = "kopia/${instanceName}/${name}";
                    StateDirectoryMode = "0700";
                    LogsDirectory = "kopia/${instanceName}/${name}";
                    LogsDirectoryMode = "0700";
                    LoadCredential = [
                      "password:${generator.files.password.path}"
                      "access-key:${keys.files."access-key".path}"
                      "secret-access-key:${keys.files."secret-access-key".path}"
                      "endpoint:${storage.files.endpoint.path}"
                      "region:${storage.files.region.path}"
                    ];
                    Nice = mkDefault 19;
                    CPUWeight = mkDefault 10;
                    IOSchedulingClass = mkDefault "idle";
                    IOWeight = mkDefault 10;
                    PrivateTmp = true;
                    NoNewPrivileges = true;
                    ProtectSystem = "strict";
                  };
                };

                mkTimer = schedule: {
                  wantedBy = [ "timers.target" ];
                  timerConfig = {
                    OnCalendar = schedule;
                    Persistent = true;
                    RandomizedDelaySec = "30m";
                  };
                };
              in
              {
                assertion = {
                  assertion = sources != [ ];
                  message = "kopia instance `${instanceName}`: user `${name}` on ${config.networking.hostName} has no sources; home-manager does not know this user, set `home.path` or add `sources`.";
                };

                generator = nameValuePair unit {
                  files.password.secret = true;
                  runtimeInputs = [ pkgs.openssl ];
                  script = ''
                    openssl rand -base64 36 | tr -d '\n' > "$out/password"
                  '';
                };

                package = wrapper;

                services."${unit}-backup" = mkService "backup" backupScript {
                  RequiresMountsFor = map (s: s.path) (filter (s: s.command == null) sources);
                };

                timers."${unit}-backup" = mkTimer cfg.schedule;
              };

            users = listToAttrs (map (name: nameValuePair name (mkUser name (userCfg name))) userNames);
          in
          {
            assertions = [
              {
                assertion = userNames != [ ];
                message = "kopia instance `${instanceName}` has no users on ${config.networking.hostName}.";
              }
            ]
            ++ mapAttrsToList (_: u: u.assertion) users;

            clan.core.vars.generators = {
              "kopia-${instanceName}-storage" = {
                share = true;
                prompts = {
                  endpoint = {
                    description = "S3 endpoint host of kopia `${instanceName}`, without a scheme.";
                    type = "hidden";
                    persist = true;
                  };
                  region = {
                    description = "S3 region of kopia `${instanceName}`, empty for providers that ignore it.";
                    type = "hidden";
                    persist = true;
                  };
                };
              };

              "kopia-${instanceName}".prompts = {
                access-key = {
                  description = "S3 access key ID for kopia `${instanceName}` on ${config.networking.hostName}.";
                  type = "hidden";
                  persist = true;
                };
                secret-access-key = {
                  description = "S3 secret access key for kopia `${instanceName}` on ${config.networking.hostName}.";
                  type = "hidden";
                  persist = true;
                };
              };
            }
            // listToAttrs (mapAttrsToList (_: u: u.generator) users);

            environment.systemPackages = mapAttrsToList (_: u: u.package) users;

            ${namespace}.services.kopia.repositories = map (name: {
              instance = instanceName;
              user = name;
              prefix = "${host}/${name}/";
            }) userNames;

            systemd.services = mergeAttrsList (mapAttrsToList (_: u: u.services) users);
            systemd.timers = mergeAttrsList (mapAttrsToList (_: u: u.timers) users);
          };
      };
  };

  perMachine = _: {
    nixosModule =
      { namespace, ... }:
      {
        ${namespace}.system.impermanence.directories = [
          "/var/cache/kopia"
          "/var/lib/kopia"
        ];
      };
  };
}
