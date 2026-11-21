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
    mkForce
    concatStringsSep
    optionalAttrs
    ;
  cfg = config.${namespace}.services.monitoring.prometheus.exporters;

  supportedFilesystems = builtins.attrNames config.boot.supportedFilesystems;
  zfsEnabled = builtins.elem "zfs" supportedFilesystems;
  nvidiaEnabled = builtins.elem "nvidia" config.services.xserver.videoDrivers;

  nvidiaSmi = "${config.hardware.nvidia.package.bin}/bin/nvidia-smi";
  nvidiaSmiGated = pkgs.writeShellScript "nvidia-smi-gated" ''
    case "$*" in
      *--help-query*) exec ${nvidiaSmi} "$@" ;;
    esac
    for d in /sys/bus/pci/devices/*; do
      read -r vendor < "$d/vendor" || continue
      [ "$vendor" = "0x10de" ] || continue
      case "$(cat "$d/class")" in 0x03*) ;; *) continue ;; esac
      [ "$(cat "$d/power/runtime_status")" = "active" ] || exit 1
    done
    exec ${nvidiaSmi} "$@"
  '';

  textfileDir = "/run/prometheus-node-exporter";

  tracefs = "/sys/kernel/tracing";
  ddtInstance = "${tracefs}/instances/zfs-ddt";
  ddtEvent = "${ddtInstance}/events/zfs_ddt/zio_read";

  ddtProbeStart = pkgs.writeShellScript "zfs-ddt-probe-start" ''
    set -e
    grep -q '^p:zfs_ddt/zio_read ' ${tracefs}/kprobe_events ||
      echo 'p:zfs_ddt/zio_read zio_read type=+54(%dx):u8 size=%r8:u64' >> ${tracefs}/kprobe_events
    mkdir -p ${ddtInstance}
    echo 4 > ${ddtInstance}/buffer_size_kb
    echo 'type == 42' > ${ddtEvent}/filter
    echo 'hist:keys=type:vals=hitcount,size if type == 42' > ${ddtEvent}/trigger
    echo 1 > ${ddtEvent}/enable
  '';

  ddtProbeStop = pkgs.writeShellScript "zfs-ddt-probe-stop" ''
    echo 0 > ${ddtEvent}/enable
    echo '!hist:keys=type:vals=hitcount,size if type == 42' > ${ddtEvent}/trigger
    rmdir ${ddtInstance}
    echo '-:zfs_ddt/zio_read' >> ${tracefs}/kprobe_events
  '';

  ddtTextfile = pkgs.writeShellScript "zfs-ddt-textfile" ''
    out=${textfileDir}/zfs_ddt.prom
    {
      echo '# TYPE zfs_ddt_lookups_total counter'
      echo '# TYPE zfs_ddt_log_entries gauge'
      for f in /proc/spl/kstat/zfs/*/ddt_stats_*; do
        [ -e "$f" ] || continue
        pool=''${f#/proc/spl/kstat/zfs/}
        ${pkgs.gawk}/bin/awk -v pool="''${pool%%/*}" -v cs="''${f##*/ddt_stats_}" '
          NR > 2 { v[$1] = $3; if ($3 != 0) nz = 1 }
          END {
            if (!nz) exit
            for (k in v) {
              l = "pool=\"" pool "\",checksum=\"" cs "\""
              if (k == "lookup")
                printf "zfs_ddt_lookups_total{%s,kind=\"all\"} %s\n", l, v[k]
              else if (k ~ /^lookup_/)
                printf "zfs_ddt_lookups_total{%s,kind=\"%s\"} %s\n", l, substr(k, 8), v[k]
              else if (k ~ /^log_(active|flushing)_entries$/)
                printf "zfs_ddt_log_entries{%s,state=\"%s\"} %s\n", l, substr(k, 5, length(k) - 12), v[k]
            }
          }' "$f"
      done
      if [ -r ${ddtEvent}/hist ]; then
        echo '# TYPE zfs_ddt_disk_reads_total counter'
        echo '# TYPE zfs_ddt_disk_read_bytes_total counter'
        ${pkgs.gawk}/bin/awk '
          /type: *42 }/ { for (i = 1; i < NF; i++) { if ($i == "hitcount:") h = $(i + 1); if ($i == "size:") s = $(i + 1) } }
          END { printf "zfs_ddt_disk_reads_total %d\nzfs_ddt_disk_read_bytes_total %d\n", h, s }' ${ddtEvent}/hist
      fi
    } > "$out.tmp"
    mv "$out.tmp" "$out"
  '';

  memoryTextfile = pkgs.writeShellScript "memory-textfile" ''
    out=${textfileDir}/memory.prom
    cg=/sys/fs/cgroup
    {
      if [ -d /sys/kernel/debug/zswap ]; then
        for f in /sys/kernel/debug/zswap/*; do
          echo "zswap $(basename "$f") $(cat "$f")"
        done
      fi
      if [ -r /sys/kernel/debug/ttm/page_pool ]; then
        ${pkgs.gawk}/bin/awk -F: '$1 ~ /^(wc|uc)( 32)?[ \t]*$/ { c = $1; gsub(/[ \t]+$/, "", c); gsub(/ /, "", c); n = split($2, v, " "); for (i = 1; i <= n; i++) print "ttm", c, i - 1, v[i] }' /sys/kernel/debug/ttm/page_pool
      fi
      for d in $cg/*.slice $cg/system.slice/*.service \
        $cg/user.slice/user-*.slice/user@*.service/*.slice \
        $cg/user.slice/user-*.slice/user@*.service/app.slice/* \
        $cg/user.slice/user-*.slice/user@*.service/app.slice/app-graphical.slice/*; do
        [ -r "$d/memory.stat" ] || continue
        n=$(basename "$d")
        echo "cur $n $(cat "$d/memory.current")"
        echo "swap $n $(cat "$d/memory.swap.current" 2>/dev/null || echo 0)"
        while read -r k v; do echo "stat $n $k $v"; done < "$d/memory.stat"
        while read -r k v; do echo "event $n $k $v"; done < "$d/memory.events"
      done
    } | ${pkgs.gawk}/bin/awk '
      function norm(n) {
        gsub(/\\x2d/, "-", n)
        gsub(/\\/, "", n)
        sub(/-[0-9a-f]{8}\.scope$/, ".scope", n)
        sub(/-[0-9]+(-[0-9]+)?\.scope$/, ".scope", n)
        sub(/^run-[a-z0-9-]+\.service$/, "run.service", n)
        sub(/^session-[0-9]+\.scope$/, "session.scope", n)
        return n
      }
      $1 == "zswap" {
        if ($2 ~ /^(stored_|pool_total_size)/) g[sprintf("zswap_%s", $2)] = $3
        else c[sprintf("zswap_events_total{event=\"%s\"}", $2)] = $3
      }
      $1 == "ttm" { g[sprintf("ttm_pool_pages{caching=\"%s\",order=\"%s\"}", $2, $3)] = $4 }
      $1 == "cur" { g[sprintf("cgroup_memory_current_bytes{cgroup=\"%s\"}", norm($2))] += $3 }
      $1 == "swap" { g[sprintf("cgroup_memory_swap_bytes{cgroup=\"%s\"}", norm($2))] += $3 }
      $1 == "stat" && $3 ~ /^(anon|file|shmem|kernel|sock|slab|pagetables|zswap|zswapped|file_mapped|file_dirty|anon_thp|active_file|inactive_file)$/ {
        g[sprintf("cgroup_memory_stat_bytes{cgroup=\"%s\",stat=\"%s\"}", norm($2), $3)] += $4
      }
      $1 == "stat" && $3 ~ /^(pgscan|pgsteal|pgmajfault|workingset_refault_anon|workingset_refault_file|zswpout|zswpin|zswpwb|pswpin|pswpout|thp_fault_alloc)$/ {
        c[sprintf("cgroup_memory_vmevents_total{cgroup=\"%s\",event=\"%s\"}", norm($2), $3)] += $4
      }
      $1 == "event" && $3 ~ /^(low|high|max|oom|oom_kill)$/ {
        c[sprintf("cgroup_memory_events_total{cgroup=\"%s\",event=\"%s\"}", norm($2), $3)] += $4
      }
      END {
        for (k in g) print k, g[k]
        for (k in c) print k, c[k]
      }' > "$out.tmp"
    mv "$out.tmp" "$out"
  '';
in
{
  options.${namespace}.services.monitoring.prometheus.exporters = {
    enable = mkEnableOption "managing prometheus exporters";
    zfs.enable = mkEnableOption "zfs exporter" // {
      default = zfsEnabled;
    };
    nvidia.enable = mkEnableOption "nvidia exporter" // {
      default = nvidiaEnabled;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} =
      let
        exporters = config.services.prometheus.exporters;
      in
      {
        ports = {
          node-exporter = exporters.node.port;
          smartctl-exporter = exporters.smartctl.port;
        }
        // optionalAttrs cfg.zfs.enable { zfs-exporter = exporters.zfs.port; }
        // optionalAttrs cfg.nvidia.enable { nvidia-exporter = exporters.nvidia-gpu.port; };

        services.monitoring.agent.targets = {
          node.port = exporters.node.port;
          smartctl = {
            inherit (exporters.smartctl) port;
            interval = exporters.smartctl.maxInterval;
          };
        }
        // optionalAttrs cfg.zfs.enable { zfs.port = exporters.zfs.port; }
        // optionalAttrs cfg.nvidia.enable { nvidia-gpu.port = exporters.nvidia-gpu.port; };
      };

    services.prometheus.exporters = {
      node = {
        enable = true;

        enabledCollectors = [
          "systemd"
          "processes"
          "ethtool"
          "softirqs"
          "tcpstat"
          "wifi"
          "interrupts"
          "buddyinfo"
          "zoneinfo"
        ];

        extraFlags = [
          "--collector.textfile.directory=${textfileDir}"
          "--collector.vmstat.fields=^(oom_kill|pgpg|pswp|pg.*fault|zswp.*|pgscan_.*|pgsteal_.*|allocstall_.*|compact_.*|thp_fault_.*|thp_collapse_.*|kswapd_.*|pageoutrun|direct_map_.*|workingset_.*|nr_gpu_.*|nr_kernel_file_pages).*"
        ];
      };

      zfs = {
        enable = cfg.zfs.enable;
        extraFlags = [
          "--collector.dataset-snapshot"
          "--properties.dataset-snapshot=used,written,creation"
          "--properties.dataset-filesystem=available,logicalused,quota,referenced,used,usedbydataset,usedbysnapshots,written"
        ];
      };

      smartctl.enable = true;

      nvidia-gpu.enable = cfg.nvidia.enable;
    };

    systemd.services.prometheus-nvidia-gpu-exporter.serviceConfig = mkIf cfg.nvidia.enable {
      ExecStart =
        let
          nvcfg = config.services.prometheus.exporters.nvidia-gpu;
        in
        mkForce ''
          ${pkgs.prometheus-nvidia-gpu-exporter}/bin/nvidia_gpu_exporter \
            --web.listen-address ${nvcfg.listenAddress}:${toString nvcfg.port} \
            --nvidia-smi-command ${nvidiaSmiGated} \
            ${concatStringsSep " " nvcfg.extraFlags}
        '';
      LogFilterPatterns = "~failed to collect metrics";
    };

    systemd.tmpfiles.rules = [ "d ${textfileDir} 0755 root root -" ];

    systemd.services.zfs-ddt-probe = mkIf cfg.zfs.enable {
      description = "Count DDT reads from disk with a tracefs histogram";
      wantedBy = [ "multi-user.target" ];
      after = [
        "zfs-import.target"
        "sys-kernel-tracing.mount"
      ];
      wants = [ "sys-kernel-tracing.mount" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecCondition = "${pkgs.coreutils}/bin/test -e ${tracefs}/events/sched/sched_switch/hist";
        ExecStart = ddtProbeStart;
        ExecStop = ddtProbeStop;
      };
    };

    systemd.services.zfs-ddt-textfile = mkIf cfg.zfs.enable {
      description = "Write DDT metrics for the node exporter textfile collector";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = ddtTextfile;
      };
    };

    systemd.services.memory-textfile = {
      description = "Write memory internals for the node exporter textfile collector";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = memoryTextfile;
      };
    };

    systemd.timers.memory-textfile = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "30s";
        OnUnitActiveSec = "15s";
        AccuracySec = "1s";
      };
    };

    systemd.timers.zfs-ddt-textfile = mkIf cfg.zfs.enable {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "1min";
        OnUnitActiveSec = "1min";
      };
    };
  };
}
