{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    any
    concatMap
    elem
    filter
    findFirst
    hasPrefix
    head
    mkOption
    types
    unique
    ;

  inherit (config.hardware.facter) report;

  cpus = report.hardware.cpu or [ ];
  gpus = report.hardware.graphics_card or [ ];
  ifaces = report.hardware.network_interface or [ ];
  mems = report.hardware.memory or [ ];

  cpu0 = if cpus == [ ] then { } else head cpus;
  cpuVendor = cpu0.vendor_name or null;
  cpuFamily = cpu0.family or null;
  cpuModel = cpu0.model or null;

  gpuDrivers = unique (concatMap (g: g.driver_modules or [ ]) gpus);
  hasGpu = drivers: any (d: elem d gpuDrivers) drivers;
  gpuDriversFor = drivers: filter (d: elem d gpuDrivers) drivers;

  intelGpuDrivers = [
    "i915"
    "xe"
  ];
  amdGpuDrivers = [ "amdgpu" ];
  nvidiaGpuDrivers = [
    "nvidia"
    "nouveau"
  ];

  # zenpower3 supports Family 17h (all models) and Family 19h Model < 0x60.
  # See zenpower.c PCI ID list: 0x1463, 0x15eb, 0x1493, 0x144b, 0x1443 (17h)
  # and 0x1653, 0x167c, 0x166d (19h Model 0/40/50). Anything 19h ≥ 0x60
  # (Raphael/Phoenix/Hawk Point) and Family 1Ah (Zen 5) are not supported.
  zenpowerSupported =
    cpuVendor == "AuthenticAMD"
    && (cpuFamily == 23 || (cpuFamily == 25 && cpuModel != null && cpuModel < 96));

  # Detect interface kind by name. PCI sub_class is unreliable: facter
  # misclassifies the MediaTek MT7925 (PCI class 0x0280, "Network controller
  # / Other") as "Ethernet controller", which leaks its driver into the
  # ethernet bucket. The systemd predictable naming scheme (`wl*` for WLAN,
  # `en*`/`eth*` for wired) is unambiguous and works across all hosts.
  isLoopback = i: (i.sub_class.name or "") == "Loopback";
  isWifiIface = i: any (hasPrefix "wl") (i.unix_device_names or [ ]);
  isEthIface = i: !(isLoopback i) && !(isWifiIface i);

  wifiIfaceList = filter isWifiIface ifaces;
  ethIfaceList = filter isEthIface ifaces;

  driversOf = list: unique (concatMap (e: e.driver_modules or [ ]) list);
  ifaceNames = list: concatMap (i: i.unix_device_names or [ ]) list;

  mainMem = findFirst (m: (m.model or "") == "Main Memory") null mems;
  mainPhys =
    if mainMem == null then
      null
    else
      findFirst (r: (r.type or "") == "phys_mem") null (mainMem.resources or [ ]);
  totalBytes = if mainPhys == null then 0 else mainPhys.range or 0;
in
{
  options.${namespace}.facts = {
    cpu = {
      manufacturer = mkOption {
        type = types.enum [
          "amd"
          "intel"
        ];
        default =
          if cpuVendor == "GenuineIntel" then
            "intel"
          else if cpuVendor == "AuthenticAMD" then
            "amd"
          else
            null;
        defaultText = "Derived from facter report.";
        description = "CPU manufacturer derived from the facter report.";
      };

      amd.zenpowerSupported = mkOption {
        type = types.bool;
        default = zenpowerSupported;
        defaultText = "Derived from facter CPU family/model.";
        description = ''
          Whether zenpower3 supports this CPU. True for AMD Family 17h
          (Zen 1/2/3) and Family 19h Model < 0x60 (Zen 3, excluding Zen 4
          Raphael/Phoenix/Hawk Point). Family 1Ah (Zen 5) is not supported.
        '';
      };
    };

    network = {
      wifi = {
        exists = mkOption {
          type = types.bool;
          default = wifiIfaceList != [ ];
          defaultText = "Derived from facter report.";
          description = "Whether the host has a WiFi network interface.";
        };
        drivers = mkOption {
          type = types.listOf types.str;
          default = driversOf wifiIfaceList;
          defaultText = "Derived from facter report.";
          description = "Kernel modules bound to WiFi network interfaces.";
        };
        interfaces = mkOption {
          type = types.listOf types.str;
          default = ifaceNames wifiIfaceList;
          defaultText = "Derived from facter report.";
          description = "WiFi network interface names (e.g. wlp4s0).";
        };
      };

      ethernet = {
        exists = mkOption {
          type = types.bool;
          default = ethIfaceList != [ ];
          defaultText = "Derived from facter report.";
          description = "Whether the host has a wired Ethernet interface.";
        };
        drivers = mkOption {
          type = types.listOf types.str;
          default = driversOf ethIfaceList;
          defaultText = "Derived from facter report.";
          description = "Kernel modules bound to Ethernet network interfaces.";
        };
        interfaces = mkOption {
          type = types.listOf types.str;
          default = ifaceNames ethIfaceList;
          defaultText = "Derived from facter report.";
          description = "Ethernet network interface names (e.g. enp3s0).";
        };
      };
    };

    gpu =
      let
        mkVendor =
          {
            drivers,
            exists ? hasGpu drivers,
            existsText ? "Derived from facter report.",
          }:
          {
            exists = mkOption {
              type = types.bool;
              default = exists;
              defaultText = existsText;
            };
            drivers = mkOption {
              type = types.listOf types.str;
              default = gpuDriversFor drivers;
              defaultText = "Derived from facter report.";
            };
          };
      in
      {
        intel = mkVendor { drivers = intelGpuDrivers; };
        amd = mkVendor {
          drivers = amdGpuDrivers;
          exists = config.hardware.facter.detected.graphics.amd.enable;
          existsText = "hardware.facter.detected.graphics.amd.enable";
        };
        nvidia = mkVendor { drivers = nvidiaGpuDrivers; };
      };

    memory = {
      totalBytes = mkOption {
        type = types.int;
        default = totalBytes;
        defaultText = "Derived from facter report.";
        description = "Total RAM in bytes (0 if unknown).";
      };
      totalMiB = mkOption {
        type = types.int;
        default = totalBytes / 1024 / 1024;
        defaultText = "Derived from facter report.";
        description = "Total RAM in MiB (0 if unknown).";
      };
    };

    system.virtualised = mkOption {
      type = types.bool;
      default = !config.hardware.facter.detected.virtualisation.none.enable;
      defaultText = "!hardware.facter.detected.virtualisation.none.enable";
      description = ''
        Whether the host runs under a hypervisor. Derived from
        `hardware.facter.detected.virtualisation.none.enable`, which facter
        sets from systemd-detect-virt (granular per-hypervisor: qemu, kvm,
        hyperv, oracle, parallels).
      '';
    };
  };
}
