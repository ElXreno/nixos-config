{
  config,
  namespace,
  lib,
  virtual,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.htop;
in
{
  options.${namespace}.programs.htop = {
    enable = mkEnableOption "Whether or not to manage htop.";
    zfsSupport = mkEnableOption "Whether or not to enable ZFS support." // {
      default = true; # TODO: Handle it by other way, proper way
    };
  };

  config = mkIf cfg.enable {
    programs.htop = {
      enable = true;
      settings = lib.mkMerge [
        {
          config_reader_min_version = 3;
          detailed_cpu_time = true;
          header_margin = true;
          hide_kernel_threads = false;
          hide_userland_threads = true;
          highlight_base_name = true;
          show_cpu_usage = true;
          show_program_path = true;
          header_layout = "two_50_50";
          column_meters_0 = [
            "LeftCPUs2"
            "Memory"
            "Swap"
            "Zram"
          ]
          ++ lib.optionals cfg.zfsSupport [
            "ZFSARC"
            "ZFSCARC"
          ];
          column_meter_modes_0 = [
            1
            1
            1
            1
          ]
          ++ lib.optionals cfg.zfsSupport [
            2
            2
          ];
          column_meters_1 = [
            "RightCPUs2"
            "Tasks"
            "LoadAverage"
            "Uptime"
            "DiskIO"
            "NetworkIO"
          ];
          column_meter_modes_1 = [
            1
            2
            2
            2
            2
            2
          ];
          # TODO: Write custom config generator
          # screen_tabs = 1;
          # "screen:I/O" = [ "PID" "USER" "IO_PRIORITY" "IO_RATE" "IO_READ_RATE" "IO_WRITE_RATE" "COMM" ];
        }
        (lib.mkIf (!virtual && !config.${namespace}.roles.server.enable) {
          show_cpu_frequency = true;
          show_cpu_temperature = true;
        })
      ];
    };
  };
}
