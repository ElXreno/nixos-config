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
    foldl'
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
  netControllers = report.hardware.network_controller or [ ];
  mems = report.hardware.memory or [ ];

  smbiosSystem = report.smbios.system or { };
  smbiosBoard = report.smbios.board or { };

  cpu0 = if cpus == [ ] then { } else head cpus;
  cpuVendor = cpu0.vendor_name or null;
  cpuFamily = cpu0.family or null;
  cpuModel = cpu0.model or null;

  gpuDrivers = unique (concatMap (g: g.driver_modules or [ ]) gpus);
  hasGpu = drivers: any (d: elem d gpuDrivers) drivers;
  gpuDriversFor = drivers: filter (d: elem d gpuDrivers) drivers;

  gpuBusId =
    drivers:
    let
      gpu = findFirst (g: any (d: elem d drivers) (g.driver_modules or [ ])) null gpus;
      function = lib.toInt (lib.last (lib.splitString "." (gpu.sysfs_bus_id or "0.0")));
    in
    if gpu == null || !(gpu ? slot) then
      ""
    else
      "PCI:${toString gpu.slot.bus}:${toString gpu.slot.number}:${toString function}";

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

  inRange = lo: hi: cpuModel != null && cpuModel >= lo && cpuModel <= hi;
  zenGeneration =
    if cpuVendor != "AuthenticAMD" then
      null
    else if cpuFamily == 23 then
      (if inRange 0 47 then 1 else 2)
    else if cpuFamily == 25 then
      (if inRange 16 31 || inRange 96 127 || inRange 160 175 then 4 else 3)
    else if cpuFamily == 26 then
      5
    else
      null;

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

  # PCI IDs in the `IWLMVM || IWLMLD` block of iwlwifi pcie/drv.c (Bz and Sc families)
  iwlwifiMldDeviceIds = [
    "272b"
    "4d40"
    "6e70"
    "7740"
    "a840"
    "d240"
    "d340"
    "e340"
    "e440"
  ];
  iwlwifiMld = any (
    c:
    elem "iwlwifi" (c.driver_modules or [ ])
    && elem (lib.toLower (c.device.hex or "")) iwlwifiMldDeviceIds
  ) netControllers;

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

      amd.zenGeneration = mkOption {
        type = types.nullOr types.ints.positive;
        default = zenGeneration;
        defaultText = "Derived from facter CPU family/model.";
        description = "Zen microarchitecture generation, null on non-AMD CPUs.";
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

      intel.bayTrail = mkOption {
        type = types.bool;
        default = cpuVendor == "GenuineIntel" && cpuFamily == 6 && cpuModel == 55;
        defaultText = "Derived from facter CPU family/model.";
        description = "Whether the CPU is a Bay Trail SoC (Intel Family 6 Model 0x37).";
      };

      cores = mkOption {
        type = types.int;
        default = foldl' (a: c: a + (c.cores or 0)) 0 cpus;
        defaultText = "Derived from facter report (sum of cpus[].cores).";
        description = "Total physical CPU cores across all sockets.";
      };

      threads = mkOption {
        type = types.int;
        default = foldl' (a: c: a + (c.siblings or c.cores or 0)) 0 cpus;
        defaultText = "Derived from facter report (sum of cpus[].siblings).";
        description = "Total logical processors (cores × SMT threads) across all sockets.";
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
        iwlwifiMld = mkOption {
          type = types.bool;
          default = iwlwifiMld;
          defaultText = "Derived from facter report.";
          description = "Whether an iwlwifi card is of the Bz or Sc family, which iwlmld drives and which needs LAR.";
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
            busId = mkOption {
              type = types.str;
              default = gpuBusId drivers;
              defaultText = "Derived from facter report.";
              description = "Decimal `PCI:bus:device:function` of the first matching GPU, empty if none.";
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

    system = {
      virtualised = mkOption {
        type = types.bool;
        default = report != { } && !config.hardware.facter.detected.virtualisation.none.enable;
        defaultText = "report != { } && !hardware.facter.detected.virtualisation.none.enable";
        description = ''
          Whether the host runs under a hypervisor. Derived from
          `hardware.facter.detected.virtualisation.none.enable`, which facter
          sets from systemd-detect-virt (granular per-hypervisor: qemu, kvm,
          hyperv, oracle, parallels). A host without a facter report counts
          as bare metal.
        '';
      };

      vendor = mkOption {
        type = types.nullOr types.str;
        default = smbiosSystem.manufacturer or null;
        defaultText = "Derived from facter report.";
        description = "SMBIOS system manufacturer, e.g. `HONOR`.";
      };

      board = mkOption {
        type = types.nullOr types.str;
        default = smbiosBoard.product or null;
        defaultText = "Derived from facter report.";
        description = "SMBIOS baseboard product, e.g. `FA507UV`.";
      };
    };
  };
}
