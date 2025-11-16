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
    ;
  cfg = config.${namespace}.services.pipewire;
in
{
  options.${namespace}.services.pipewire = {
    enable = mkEnableOption "Whether or not to manage pipewire.";
    rnnoise = {
      enable = mkEnableOption "Whether to enable RNNoise.";
      mic = mkOption {
        type = types.str;
        default = "alsa_input.target";
      };
    };
  };

  config = mkIf cfg.enable {
    security.rtkit.enable = true;
    services.pipewire = {
      enable = true;
      alsa.enable = true;
      alsa.support32Bit = true;
      pulse.enable = true;

      extraLv2Packages = [ pkgs.lsp-plugins ];

      extraConfig = {
        client."80-resample"."stream.properties"."resample.quality" = 14;
        pipewire-pulse."80-resample"."stream.properties"."resample.quality" = 14;
        pipewire = {
          "80-low-latency"."context.properties" = {
            "default.clock.quantum" = 512;
            "default.clock.min-quantum" = 512;
            "default.clock.max-quantum" = 512;
          };
          "80-allowed-rates"."context.properties"."default.clock.allowed-rates" = [
            44100
            48000
            96000
            192000
          ];
          "99-noise-cancelling" = mkIf cfg.rnnoise.enable {
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
                    "node.name" = "rnnoise.input";
                    "node.passive" = true;
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                    "target.object" = cfg.rnnoise.mic;
                  };
                  "playback.props" = {
                    "node.name" = "rnnoise.source";
                    "media.class" = "Audio/Source";
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                  };
                };
              }
              {
                "name" = "libpipewire-module-filter-chain";
                "args" = {
                  "node.description" = "Voice Enhanced source";
                  "media.name" = "Voice Enhanced source";
                  "filter.graph" = {
                    "nodes" = [
                      {
                        "type" = "ladspa";
                        "name" = "comp";
                        "plugin" = "${pkgs.ladspaPlugins}/lib/ladspa/sc4_1882.so";
                        "label" = "sc4";
                        "control" = {
                          "Attack time (ms)" = 10.6;
                          "Release time (ms)" = 500.0;
                          "Threshold level (dB)" = -18.3;
                          "Ratio (1:n)" = 4.0;
                          "Knee radius (dB)" = 3.0;
                          "Makeup gain (dB)" = 6.0;
                        };
                      }
                    ];
                  };
                  "capture.props" = {
                    "node.name" = "voicecomp.input";
                    "node.passive" = true;
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                    "target.object" = "rnnoise.source";
                  };
                  "playback.props" = {
                    "node.name" = "voice.source";
                    "media.class" = "Audio/Source";
                    "audio.rate" = 48000;
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
