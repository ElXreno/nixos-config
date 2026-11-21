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
    getExe
    getExe'
    toLower
    mapAttrs'
    mapAttrsToList
    nameValuePair
    mergeAttrsList
    concatMap
    optional
    optionalAttrs
    unique
    ;
  cfg = config.${namespace}.services.zerofs;
  host = config.networking.hostName;
  generators = config.clan.core.vars.generators;
  keys = generators.zerofs;
  fusermount = "/run/wrappers/bin/fusermount3";

  mkVolume =
    name: vol:
    let
      unit = "zerofs-${name}";
      group = config.users.users.${vol.user}.group;
      runtimeDir = "/run/${unit}";
      ninepSocket = "${runtimeDir}/9p.sock";

      configFile = (pkgs.formats.toml { }).generate "${unit}.toml" {
        cache = {
          inherit (vol.cache) dir;
          disk_size_gb = vol.cache.sizeGb * 1.0;
          memory_size_gb = 4.0;
          warm_metadata = "full";
        };
        storage = {
          url = "s3://zerofs/${toLower host}-${name}";
          encryption_password = "\${ZEROFS_PASSWORD}";
        };
        aws = {
          access_key_id = "\${AWS_ACCESS_KEY_ID}";
          secret_access_key = "\${AWS_SECRET_ACCESS_KEY}";
          endpoint = "\${ZEROFS_S3_ENDPOINT}";
          region = "\${ZEROFS_S3_REGION}";
          conditional_put = "redis://127.0.0.1:${toString vol.lockPort}";
        };
        filesystem.compression = vol.compression;
        servers = {
          ninep.unix_socket = ninepSocket;
          rpc.unix_socket = "${runtimeDir}/rpc.sock";
        };
        prometheus.addresses = [ "127.0.0.1:${toString vol.metricsPort}" ];
        telemetry.enabled = false;
      };

      server = pkgs.writeShellApplication {
        name = "${unit}-server";
        runtimeInputs = [ pkgs.zerofs ];
        text = ''
          AWS_ACCESS_KEY_ID="$(< "$CREDENTIALS_DIRECTORY/access-key")"
          AWS_SECRET_ACCESS_KEY="$(< "$CREDENTIALS_DIRECTORY/secret-access-key")"
          ZEROFS_PASSWORD="$(< "$CREDENTIALS_DIRECTORY/password")"
          ZEROFS_S3_ENDPOINT="https://$(< "$CREDENTIALS_DIRECTORY/endpoint")"
          ZEROFS_S3_REGION="$(< "$CREDENTIALS_DIRECTORY/region")"
          export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY ZEROFS_PASSWORD ZEROFS_S3_ENDPOINT ZEROFS_S3_REGION
          exec zerofs run --config ${configFile}
        '';
      };

      waitForSocket = pkgs.writeShellApplication {
        name = "${unit}-wait-for-socket";
        text = ''
          until [ -S ${ninepSocket} ]; do
            sleep 1
          done
        '';
      };

      control = pkgs.writeShellApplication {
        name = unit;
        runtimeInputs = [ pkgs.zerofs ];
        text = ''
          export AWS_ACCESS_KEY_ID=unused AWS_SECRET_ACCESS_KEY=unused ZEROFS_PASSWORD=unused
          export ZEROFS_S3_ENDPOINT=unused ZEROFS_S3_REGION=unused
          exec zerofs "$@" --config ${configFile}
        '';
      };

      archiveOwn = pkgs.writeShellApplication {
        name = "${unit}-archive-own";
        runtimeInputs = [
          pkgs.coreutils
          pkgs.fd
        ];
        text = ''
          src=${toString vol.archive.source}
          cutoff=$(date -d "-${toString vol.archive.olderThanDays} days" +%F)

          for dir in "$src"/*/*; do
            if [ ! -d "$dir" ] || [ -L "$dir" ]; then continue; fi
            [[ ''${dir##*/} =~ ^([0-9]{4}-[0-9]{2}-[0-9]{2})- ]] || continue
            [[ ''${BASH_REMATCH[1]} < $cutoff ]] || continue
            chown -h ${vol.user}:${group} "$dir"
            fd --hidden --no-ignore --owner '!${vol.user}' . "$dir" -X chown -h ${vol.user}:${group}
          done
        '';
      };

      archive = pkgs.writeShellApplication {
        name = "${unit}-archive";
        runtimeInputs = [
          control
          pkgs.coreutils
          pkgs.fd
          pkgs.findutils
          pkgs.rsync
          pkgs.util-linux
        ];
        text = ''
          src=${toString vol.archive.source}
          dst=${vol.mountPoint}/${baseNameOf (toString vol.archive.source)}
          dry=0
          [ "''${1:-}" = "--dry-run" ] && dry=1

          mountpoint -q ${vol.mountPoint} || { echo "${vol.mountPoint} is not mounted"; exit 1; }
        ''
        + (
          if vol.archive.type == "files" then
            ''
              work=$(mktemp -d)
              trap 'rm -rf "$work"' EXIT
              fd --type f --hidden --no-ignore --changed-before ${toString vol.archive.olderThanDays}d \
                --base-directory "$src" --print0 . > "$work/list"
              count=$(tr -cd '\0' < "$work/list" | wc -c)
              echo "$count files older than ${toString vol.archive.olderThanDays} days"
              [ "$count" -gt 0 ] || exit 0

              if [ "$dry" = 1 ]; then
                (cd "$src" && du --files0-from="$work/list" --total --block-size=1M | tail -n 1)
                exit 0
              fi

              move_chunk() {
                local list=$1
                rsync -aHS --numeric-ids --no-implied-dirs --from0 --files-from="$list" "$src/" "$dst/" \
                  || { echo "copy failed: $list"; return 1; }
                ${unit} flush > /dev/null || { echo "flush failed: $list"; return 1; }
                if [ -n "$(rsync -aHS --numeric-ids --no-implied-dirs --dry-run --itemize-changes \
                  --from0 --files-from="$list" "$src/" "$dst/")" ]; then
                  echo "verification failed: $list"
                  return 1
                fi
                (cd "$src" && xargs -0 rm -f -- < "$list")
              }
              export -f move_chunk
              export src dst

              mkdir -p "$dst"
              split --separator='\0' --number=l/${toString vol.archive.parallel} "$work/list" "$work/chunk."
              # shellcheck disable=SC2016
              printf '%s\0' "$work"/chunk.* \
                | xargs -0 -n 1 -P ${toString vol.archive.parallel} ${pkgs.runtimeShell} -c 'move_chunk "$1"' _
              echo "moved $count files"
            ''
          else
            ''
              cutoff=$(date -d "-${toString vol.archive.olderThanDays} days" +%F)

              for left in "$src"/*/*.archiving; do
                [ -d "$left" ] || continue
                if [ -L "''${left%.archiving}" ]; then
                  chmod -R u+w "$left"
                  rm -rf "$left"
                fi
              done

              candidates=()
              for dir in "$src"/*/*; do
                if [ ! -d "$dir" ] || [ -L "$dir" ]; then continue; fi
                name=''${dir##*/}
                [[ $name == *.archiving ]] && continue
                [[ $name =~ ^([0-9]{4}-[0-9]{2}-[0-9]{2})- ]] || continue
                [[ ''${BASH_REMATCH[1]} < $cutoff ]] || continue
                [ -n "$(fd --hidden --no-ignore --changed-within 7d --max-results 1 . "$dir")" ] && continue
                candidates+=("$dir")
              done
              echo "''${#candidates[@]} directories older than $cutoff"
              [ "''${#candidates[@]}" -gt 0 ] || exit 0

              if [ "$dry" = 1 ]; then
                du --summarize --block-size=1M --total "''${candidates[@]}" | sort --numeric-sort --reverse
                exit 0
              fi

              # shellcheck disable=SC2329
              copy_one() {
                local dir=$1 target
                target=$dst/''${dir#"$src"/}
                mkdir -p "''${target%/*}"
                rsync -aHS --numeric-ids "$dir/" "$target/" || { echo "copy failed: $dir"; return 1; }
                if [ -n "$(rsync -aHS --numeric-ids --dry-run --itemize-changes "$dir/" "$target/")" ]; then
                  echo "verification failed: $dir"
                  return 1
                fi
                printf '%s\0' "$dir" >> "$verified"
              }
              # shellcheck disable=SC2329
              finish_one() {
                local dir=$1 target tmp
                target=$dst/''${dir#"$src"/}
                tmp=$dir.archiving
                mv "$dir" "$tmp"
                ln -s "$target" "$dir"
                chmod -R u+w "$tmp"
                rm -rf "$tmp"
                echo "archived: $dir"
              }
              export -f copy_one finish_one
              verified=$(mktemp)
              trap 'rm -f "$verified"' EXIT
              export src dst verified

              failed=0
              batch=64
              for ((i = 0; i < ''${#candidates[@]}; i += batch)); do
                slice=("''${candidates[@]:i:batch}")
                : > "$verified"
                # shellcheck disable=SC2016
                printf '%s\0' "''${slice[@]}" \
                  | xargs -0 -n 1 -P ${toString vol.archive.parallel} ${pkgs.runtimeShell} -c 'copy_one "$1"' _ || failed=1
                ${unit} flush > /dev/null || { echo "flush failed"; exit 1; }
                # shellcheck disable=SC2016
                xargs -0 -n 1 -P ${toString vol.archive.parallel} ${pkgs.runtimeShell} -c 'finish_one "$1"' _ < "$verified" || failed=1
              done
              exit "$failed"
            ''
        );
      };
    in
    {
      packages = [ control ] ++ optional vol.archive.enable archive;

      tmpfiles = [
        "d ${vol.cache.dir} 0700 ${vol.user} ${group} -"
        "d ${vol.mountPoint} 0755 ${vol.user} ${group} -"
      ];

      redis.${unit} = {
        enable = true;
        port = vol.lockPort;
        save = [ ];
      };

      services = {
        ${unit} = {
          description = "ZeroFS server for ${name}";
          wants = [
            "network-online.target"
            "${unit}-mount.service"
          ];
          requires = [ "redis-${unit}.service" ];
          after = [
            "network-online.target"
            "nss-lookup.target"
            "mihomo.service"
            "redis-${unit}.service"
          ];
          wantedBy = [ "multi-user.target" ];
          unitConfig.RequiresMountsFor = [ vol.cache.dir ];
          serviceConfig = {
            User = vol.user;
            ExecStart = getExe server;
            RuntimeDirectory = unit;
            LoadCredential = [
              "access-key:${keys.files."access-key".path}"
              "secret-access-key:${keys.files."secret-access-key".path}"
              "endpoint:${keys.files.endpoint.path}"
              "region:${keys.files.region.path}"
              "password:${generators."${unit}-password".files.password.path}"
            ];
            Restart = "on-failure";
            RestartSec = 10;
            TimeoutStopSec = "5min";
          };
        };

        "${unit}-mount" = {
          description = "ZeroFS FUSE mount at ${vol.mountPoint}";
          requires = [ "${unit}.service" ];
          after = [ "${unit}.service" ];
          wantedBy = [ "multi-user.target" ];
          path = [ "/run/wrappers" ];
          serviceConfig = {
            User = vol.user;
            ExecStartPre = [
              "-${fusermount} -u -z ${vol.mountPoint}"
              (getExe waitForSocket)
            ];
            ExecStart = "${getExe' pkgs.zerofs "zerofs"} mount --writeback false unix:${ninepSocket} ${vol.mountPoint}";
            ExecStop = "${fusermount} -u ${vol.mountPoint}";
            Restart = "on-failure";
            RestartSec = 5;
            TimeoutStopSec = 20;
          };
        };
      }
      // optionalAttrs vol.archive.enable {
        "${unit}-archive" = {
          description = "Archive old session directories into ZeroFS ${name}";
          requires = [ "${unit}-mount.service" ];
          restartIfChanged = false;
          after = [ "${unit}-mount.service" ];
          serviceConfig = {
            Type = "exec";
            User = vol.user;
            ExecStartPre = mkIf (vol.archive.type == "session-dirs") "+${getExe archiveOwn}";
            ExecStart = getExe archive;
            Nice = 19;
            CPUWeight = 10;
            IOSchedulingClass = "idle";
          };
        };
      };

      timers = optionalAttrs vol.archive.enable {
        "${unit}-archive" = {
          wantedBy = [ "timers.target" ];
          timerConfig = {
            OnCalendar = "01:00";
            Persistent = true;
          };
        };
      };
    };

  volumes = mapAttrsToList mkVolume cfg.volumes;

  prompt = description: {
    inherit description;
    type = "hidden";
    persist = true;
  };
in
{
  options.${namespace}.services.zerofs.volumes = mkOption {
    default = { };
    description = "ZeroFS filesystems on the `zerofs` bucket, each under the prefix `<host>-<name>`.";
    type = types.attrsOf (
      types.submodule (
        { name, ... }:
        {
          options = {
            user = mkOption {
              type = types.str;
              default = config.${namespace}.primaryUser;
              defaultText = lib.literalExpression "config.\${namespace}.primaryUser";
              description = "User that runs the server and the FUSE client and owns the mounted files.";
            };
            mountPoint = mkOption {
              type = types.path;
              default = "/mnt/${name}-cold";
              description = "Where the filesystem is mounted.";
            };
            metricsPort = mkOption {
              type = types.port;
              description = "Local port of the Prometheus endpoint.";
            };
            lockPort = mkOption {
              type = types.port;
              description = "Local port of the Valkey instance that serialises the conditional writes of this volume; the object store does not make them atomic.";
            };
            compression = mkOption {
              type = types.str;
              default = "zstd-9";
              description = "Compression of new extents, `lz4` or `zstd-<level>`.";
            };
            cache = {
              dir = mkOption {
                type = types.path;
                default = "/mnt/scratch/zerofs-cache";
                description = "Directory of the local disk cache; must survive reboots to stay warm.";
              };
              sizeGb = mkOption {
                type = types.ints.positive;
                default = 200;
                description = "Disk budget of the local cache in decimal gigabytes.";
              };
            };
            archive = {
              enable = mkEnableOption "moving old data into this volume";
              type = mkOption {
                type = types.enum [
                  "session-dirs"
                  "files"
                ];
                description = ''
                  `session-dirs` treats `<project>/<YYYY-MM-DD>-<topic>` directories under `source`
                  as units, ages them by the date in the name and leaves a symlink behind.
                  `files` moves every file under `source` whose modification time is old enough
                  and leaves nothing behind.
                '';
              };
              source = mkOption {
                type = types.nullOr types.path;
                default = null;
                description = ''
                  Tree of `<project>/<YYYY-MM-DD>-<topic>` directories. A directory older than
                  `olderThanDays` is copied to the same relative path under the mount point,
                  verified, and replaced by a symlink.
                '';
              };
              olderThanDays = mkOption {
                type = types.ints.positive;
                default = 30;
                description = "Age, by the date in the directory name, after which a directory is archived.";
              };
              parallel = mkOption {
                type = types.ints.positive;
                default = 4;
                description = "Directories archived at the same time.";
              };
            };
          };
        }
      )
    );
  };

  config = mkIf (cfg.volumes != { }) {
    assertions = mapAttrsToList (name: vol: {
      assertion = vol.archive.enable -> vol.archive.source != null;
      message = "zerofs volume `${name}`: `archive.source` must be set when `archive.enable` is on.";
    }) cfg.volumes;

    clan.core.vars.generators = {
      zerofs.prompts = {
        access-key = prompt "S3 access key ID for zerofs on ${host}.";
        secret-access-key = prompt "S3 secret access key for zerofs on ${host}.";
        endpoint = prompt "S3 endpoint host for zerofs on ${host}, without a scheme.";
        region = prompt "S3 region for zerofs on ${host}.";
      };
    }
    // mapAttrs' (
      name: _:
      nameValuePair "zerofs-${name}-password" {
        files.password.secret = true;
        runtimeInputs = [ pkgs.openssl ];
        script = ''
          openssl rand -base64 36 | tr -d '\n' > "$out/password"
        '';
      }
    ) cfg.volumes;

    ${namespace} = {
      ports = mergeAttrsList (
        mapAttrsToList (name: vol: {
          "zerofs-${name}-metrics" = vol.metricsPort;
          "zerofs-${name}-lock" = vol.lockPort;
        }) cfg.volumes
      );
      services.monitoring.agent.targets = mapAttrs' (
        name: vol: nameValuePair "zerofs-${name}" { port = vol.metricsPort; }
      ) cfg.volumes;
    };

    services.redis = {
      package = pkgs.valkey;
      vmOverCommit = false;
      servers = mergeAttrsList (map (v: v.redis) volumes);
    };

    environment.systemPackages = concatMap (v: v.packages) volumes;
    systemd.tmpfiles.rules = unique (concatMap (v: v.tmpfiles) volumes);
    systemd.services = mergeAttrsList (map (v: v.services) volumes);
    systemd.timers = mergeAttrsList (map (v: v.timers) volumes);
  };
}
