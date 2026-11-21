{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption optionals;
  cfg = config.${namespace}.services.flydigictl;
  nvidia = config.${namespace}.facts.gpu.nvidia.exists;
  cpuSensor =
    if config.${namespace}.facts.cpu.manufacturer == "intel" then
      {
        hwmon = "coretemp";
        label = "Package id 0";
      }
    else
      {
        hwmon = "k10temp";
        label = "Tctl";
      };
in
{
  options.${namespace}.services.flydigictl = {
    enable = mkEnableOption "the Flydigi cooler fan curve daemon";
  };

  config = mkIf cfg.enable {
    services.flydigictl = {
      enable = true;
      nvidia.enable = nvidia;

      settings = {
        interval_secs = 3;
        hysteresis_rpm = 100;
        standby = "off";

        lights_follow_screens = true;

        lighting = {
          mode = {
            mode = "effect";
            effect = 5;
          };
          brightness = 50;
          indicators = true;
        };

        smoothing = {
          rise_secs = 10.0;
          fall_secs = 60.0;
          panic_c = 90;
        };

        curves = [
          {
            name = "ram";
            sensor.hwmon = "spd5118";
            panic_c = 78;
            points = [
              {
                temp_c = 46;
                rpm = 0;
              }
              {
                temp_c = 47;
                rpm = 500;
              }
              {
                temp_c = 58;
                rpm = 800;
              }
              {
                temp_c = 64;
                rpm = 1400;
              }
              {
                temp_c = 70;
                rpm = 2200;
              }
              {
                temp_c = 74;
                rpm = 3000;
              }
              {
                temp_c = 78;
                rpm = 4000;
              }
            ];
          }
          {
            name = "nvme";
            sensor = {
              hwmon = "nvme";
              label = "Composite";
            };
            panic_c = 80;
            points = [
              {
                temp_c = 44;
                rpm = 0;
              }
              {
                temp_c = 45;
                rpm = 600;
              }
              {
                temp_c = 55;
                rpm = 1200;
              }
              {
                temp_c = 65;
                rpm = 2000;
              }
              {
                temp_c = 74;
                rpm = 3000;
              }
              {
                temp_c = 83;
                rpm = 4000;
              }
            ];
          }
          {
            name = "cpu";
            sensor = cpuSensor;
            panic_c = 95;
            points = [
              {
                temp_c = 55;
                rpm = 0;
              }
              {
                temp_c = 56;
                rpm = 600;
              }
              {
                temp_c = 65;
                rpm = 1100;
              }
              {
                temp_c = 75;
                rpm = 1800;
              }
              {
                temp_c = 85;
                rpm = 2800;
              }
              {
                temp_c = 95;
                rpm = 4000;
              }
            ];
          }
        ]
        ++ optionals nvidia [
          {
            name = "gpu";
            sensor = {
              kind = "nvidia";
              label = "core";
            };
            panic_c = 87;
            points = [
              {
                temp_c = 52;
                rpm = 0;
              }
              {
                temp_c = 53;
                rpm = 600;
              }
              {
                temp_c = 62;
                rpm = 1100;
              }
              {
                temp_c = 72;
                rpm = 1900;
              }
              {
                temp_c = 80;
                rpm = 2800;
              }
              {
                temp_c = 87;
                rpm = 4000;
              }
            ];
          }
          {
            name = "vram";
            sensor = {
              kind = "nvidia";
              label = "memory";
            };
            panic_c = 92;
            points = [
              {
                temp_c = 54;
                rpm = 0;
              }
              {
                temp_c = 55;
                rpm = 600;
              }
              {
                temp_c = 66;
                rpm = 1100;
              }
              {
                temp_c = 76;
                rpm = 1900;
              }
              {
                temp_c = 84;
                rpm = 2800;
              }
              {
                temp_c = 92;
                rpm = 4000;
              }
            ];
          }
        ];
      };
    };
  };
}
