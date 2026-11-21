{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    attrValues
    concatMap
    concatMapStrings
    filterAttrs
    mapAttrs
    mapAttrsToList
    mkEnableOption
    mkIf
    optionalString
    ;
  cfg = config.${namespace}.system.hardware.peripherals;

  devices = {
    lamzu = {
      description = "Lamzu Maya X mouse and its 8K dongle";
      vendor = "373e";
      products = [
        "001c"
        "b01c"
        "001e"
        "b01e"
      ];
    };
    zaopin = {
      description = "Zaopin ZPW MAX mouse and its dongle";
      vendor = "3554";
      products = [
        "f526"
        "f527"
      ];
    };
    silakka54 = {
      description = "Silakka54 keyboard";
      vendor = "feed";
      products = [ "1212" ];
      serial = "*vial:f64c2b3c*";
      packages = [ pkgs.vial ];
    };
  };

  enabledDevices = filterAttrs (name: _: cfg.${name}.enable) devices;

  rule =
    device: product:
    ''SUBSYSTEM=="hidraw", KERNEL=="hidraw*",${
      optionalString (device ? serial) " ATTRS{serial}==\"${device.serial}\","
    } ATTRS{idVendor}=="${device.vendor}", ATTRS{idProduct}=="${product}", MODE="0660", TAG+="uaccess"'';
in
{
  options.${namespace}.system.hardware.peripherals =
    mapAttrs (_: device: {
      enable = mkEnableOption "user access to the ${device.description}";
    }) devices
    // {
      libinput.neverDebounce = mkEnableOption "the libinput quirk that disables mouse button debouncing";
    };

  config = {
    services.udev.packages = mapAttrsToList (
      name: device:
      pkgs.writeTextFile {
        name = "${name}-udev-rules";
        destination = "/lib/udev/rules.d/70-${name}.rules";
        text = concatMapStrings (product: rule device product + "\n") device.products;
      }
    ) enabledDevices;

    environment.systemPackages = concatMap (device: device.packages or [ ]) (attrValues enabledDevices);

    environment.etc."libinput/local-overrides.quirks" = mkIf cfg.libinput.neverDebounce {
      text = ''
        [Never Debounce]
        MatchUdevType=mouse
        ModelBouncingKeys=1
      '';
    };
  };
}
