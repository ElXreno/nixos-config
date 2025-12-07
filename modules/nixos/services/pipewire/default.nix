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
            "default.clock.quantum" = 1024;
            "default.clock.min-quantum" = 1024;
            "default.clock.max-quantum" = 4096;
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
                  "node.description" = "Noise Cancelled Voise";
                  "media.name" = "Noise Cancelled Voice Chain Source";
                  "filter.graph" = {
                    "nodes" = [
                      {
                        "type" = "ladspa";
                        "name" = "hpf";
                        "plugin" = "${pkgs.ladspaPlugins}/lib/ladspa/butterworth_1902.so";
                        "label" = "butthigh_iir";
                        "control" = {
                          "Cutoff Frequency (Hz)" = 120.0;
                          "Resonance" = 0.707;
                        };
                      }
                      {
                        "type" = "ladspa";
                        "name" = "gate";
                        "plugin" = "${pkgs.ladspaPlugins}/lib/ladspa/gate_1410.so";
                        "label" = "gate";
                        "control" = {
                          "LF key filter (Hz)" = 120.0;
                          "HF key filter (Hz)" = 8000.0;
                          "Threshold (dB)" = -50.0;
                          "Attack (ms)" = 1.0;
                          "Hold (ms)" = 150.0;
                          "Decay (ms)" = 200.0;
                          "Range (dB)" = -90.0;
                          "Output select (-1 = key listen, 0 = gate, 1 = bypass)" = 0;
                        };
                      }
                      {
                        type = "ladspa";
                        name = "deepfilternet";
                        plugin = "${pkgs.deepfilternet}/lib/ladspa/libdeep_filter_ladspa.so";
                        label = "deep_filter_mono";
                        control = {
                          "Attenuation Limit (dB)" = 100;
                        };
                      }
                      {
                        "type" = "ladspa";
                        "name" = "comp";
                        "plugin" = "${pkgs.ladspaPlugins}/lib/ladspa/sc4m_1916.so";
                        "label" = "sc4m";
                        "control" = {
                          "Attack time (ms)" = 5.0;
                          "Release time (ms)" = 100.0;
                          "Threshold level (dB)" = -20.0;
                          "Ratio (1:n)" = 3.0;
                          "Knee radius (dB)" = 3.0;
                          "Makeup gain (dB)" = 3.0;
                        };
                      }
                    ];
                    "links" = [
                      {
                        "output" = "hpf:Output";
                        "input" = "gate:Input";
                      }
                      {
                        "output" = "gate:Output";
                        "input" = "deepfilternet:Audio In";
                      }
                      {
                        "output" = "deepfilternet:Audio Out";
                        "input" = "comp:Input";
                      }
                    ];
                  };
                  "capture.props" = {
                    "node.name" = "voice_chain.input";
                    "node.passive" = true;
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                    "target.object" = cfg.rnnoise.mic;
                  };
                  "playback.props" = {
                    "node.name" = "voice.source";
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
