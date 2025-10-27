{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.pipewire;
in
{
  options.${namespace}.services.pipewire = {
    enable = mkEnableOption "Whether or not to manage pipewire.";
    enableRNNoise = mkEnableOption "Whether to enable RNNoise.";
  };

  config = mkIf cfg.enable {
    security.rtkit.enable = true;
    services.pipewire = {
      enable = true;
      alsa.enable = true;
      alsa.support32Bit = true;
      pulse.enable = true;

      extraConfig = {
        client."99-resample"."stream.properties"."resample.quality" = 14;
        pipewire-pulse."99-resample"."stream.properties"."resample.quality" = 14;
        pipewire = {
          "92-low-latency"."context.properties" = {
            "default.clock.quantum" = 512;
            "default.clock.min-quantum" = 512;
            "default.clock.max-quantum" = 512;
          };
          "99-allowed-rates"."context.properties"."default.clock.allowed-rates" = [
            44100
            48000
            96000
            192000
          ];
          "99-rnnoise" = mkIf cfg.enableRNNoise {
            "context.modules" = [
              {
                "name" = "libpipewire-module-filter-chain";
                "args" = {
                  "node.description" = "Noise Canceling source";
                  "media.name" = "Noise Canceling source";
                  "filter.graph" = {
                    "nodes" = [
                      {
                        "type" = "ladspa";
                        "name" = "rnnoise";
                        "plugin" = "${pkgs.rnnoise-plugin}/lib/ladspa/librnnoise_ladspa.so";
                        "label" = "noise_suppressor_mono";
                        "control" = {
                          "VAD Threshold (%)" = 90.0;
                          "VAD Grace Period (ms)" = 350;
                          "Retroactive VAD Grace (ms)" = 80;
                        };
                      }
                    ];
                  };
                  "capture.props" = {
                    "node.name" = "capture.rnnoise_source";
                    "node.passive" = true;
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                    "stream.dont-remix" = true;
                  };
                  "playback.props" = {
                    "node.name" = "rnnoise_source";
                    "media.class" = "Audio/Source";
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                  };
                };
              }
            ];
          };
        };
      };
    };

    # Real-time audio processing
    security.pam.loginLimits = [
      {
        domain = "@audio";
        item = "memlock";
        type = "-";
        value = "unlimited";
      }
      {
        domain = "@audio";
        item = "rtprio";
        type = "-";
        value = "99";
      }
      {
        domain = "@audio";
        item = "nofile";
        type = "soft";
        value = "99999";
      }
      {
        domain = "@audio";
        item = "nofile";
        type = "hard";
        value = "99999";
      }
    ];

    services.udev = {
      extraRules = ''
        KERNEL=="rtc0", GROUP="audio"
        KERNEL=="hpet", GROUP="audio"
        DEVPATH=="/devices/virtual/misc/cpu_dma_latency", OWNER="root", GROUP="audio", MODE="0660"
      '';
    };
  };
}
