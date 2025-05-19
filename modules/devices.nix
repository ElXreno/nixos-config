# Source: https://github.com/balsoft/nixos-config/blob/2f90786d28115f95976adf635f2eb7877a4bc2f5/modules/devices.nix

{ lib, config, ... }:
with lib;
with types;
{
  options = {
    device = mkOption { type = str; };
    deviceSpecific = {
      isLaptop = mkOption {
        type = bool;
        default = (builtins.match ".*Laptop" config.networking.hostName) != null;
      };
      isDesktop = mkOption {
        type = bool;
        default = (builtins.match ".*Desktop" config.networking.hostName) != null;
      };
      isServer = mkOption {
        type = bool;
        default = (builtins.match ".*Server" config.networking.hostName) != null;
      };
      isVM = mkOption {
        type = bool;
        default = (builtins.match ".*VM" config.networking.hostName) != null;
      };
      devInfo = {
        cpu = {
          arch = mkOption {
            type = enum [
              "x86_64"
              "aarch64"
            ];
          };
          vendor = mkOption {
            type = enum [
              "amd"
              "intel"
              "broadcom"
            ];
          };
          clock = mkOption { type = int; };
          cores = mkOption { type = int; };
        };
        drive = {
          type = mkOption {
            type = enum [
              "hdd"
              "ssd"
            ];
          };
          speed = mkOption { type = int; };
          size = mkOption { type = int; };
        };
        ram = mkOption { type = int; };
        legacy = mkOption {
          type = bool;
          default = false;
        };
        bigScreen = mkOption {
          type = bool;
          default = true;
        };
      };
      # Whether machine is powerful enough for heavy stuff
      goodMachine =
        with config.deviceSpecific;
        mkOption {
          type = bool;
          default =
            devInfo.cpu.clock * devInfo.cpu.cores >= 4000 && devInfo.drive.size >= 100 && devInfo.ram >= 8;
        };
      isHost = mkOption {
        type = bool;
        default = false;
      };
      bigScreen = mkOption {
        type = bool;
        default = config.deviceSpecific.devInfo ? bigScreen;
      };
      # Custom attrs
      usesPlasma = mkOption {
        type = bool;
        default = !config.deviceSpecific.isServer;
      };
      usesCustomBootloader = mkOption {
        type = bool;
        default = false;
      };
    };
  };
}
