{ lib, ... }:
with lib;
with types;
{
  options = {
    device = {
      hostname = mkOption {
        type = str;
        default = null;
      };

      isPrimary = mkEnableOption "isPrimary";

      isDesktop = mkEnableOption "isDesktop";
      isLaptop = mkEnableOption "isLaptop";
      isServer = mkEnableOption "isServer";

      isLegacy = mkEnableOption "isLegacy";

      laptop = {
        manufacturer = {
          Fujitsu = mkEnableOption "Fujitsu";
          Asus = mkEnableOption "Asus";
          Dell = mkEnableOption "Dell";
          Honor = mkEnableOption "Honor";
        };
        model = mkOption {
          type = str;
          default = null;
        };
      };

      cpu = {
        amd = mkEnableOption "amd";
        intel = mkEnableOption "intel";
      };

      gpu = {
        nvidia = mkEnableOption "nvidia";
        nvidiaLegacy = mkEnableOption "nvidiaLegacy";
        amd = mkEnableOption "amd";
        intel = mkEnableOption "intel";
      };

      network = {
        hasWirelessCard = mkEnableOption "hasWirelessCard";
        wirelessCard = mkOption {
          type = enum [
            "RTL8852BE" # Realtek
            "AX200" # Intel
          ];
          default = null;
        };
      };

      users = {
        elxreno = mkEnableOption "elxreno" // { default = true; };
        alena = mkEnableOption "alena";
      };
    };
  };
}
