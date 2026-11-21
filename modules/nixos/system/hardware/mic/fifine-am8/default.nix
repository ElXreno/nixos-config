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
    optionalString
    ;
  cfg = config.${namespace}.system.hardware.mic.fifine-am8;
in
{
  options.${namespace}.system.hardware.mic.fifine-am8 = {
    enable = mkEnableOption "managing Fifine AM8 stuff";

    vendorId = mkOption {
      type = types.str;
      default = "3142";
    };
    productId = mkOption {
      type = types.str;
      default = "a010";
    };

    disableSidetone = mkEnableOption "disabling sidetone by default" // {
      default = true;
    };

    pipewire.noise-cancelling = {
      enable = mkEnableOption "noise cancelling" // {
        default = true;
      };

      target = mkOption {
        type = types.str;
        default = "alsa_input.usb-MV-SILICON_fifine_Microphone_20190808-00.analog-stereo";
      };
    };
  };

  config = mkIf cfg.enable {
    services.udev.extraRules = ''
      ACTION=="add|change", SUBSYSTEM=="usb", ATTR{idVendor}=="${cfg.vendorId}", ATTR{idProduct}=="${cfg.productId}", ATTR{power/control}="on"

      ${optionalString cfg.disableSidetone ''
        ACTION=="add|change", SUBSYSTEM=="sound", KERNEL=="controlC*", ATTRS{idVendor}=="${cfg.vendorId}", ATTRS{idProduct}=="${cfg.productId}", RUN+="${pkgs.alsa-utils}/bin/amixer -c %n cset name='Mic Playback Switch' off"
      ''}
    '';

    services.pipewire = {
      extraLadspaPackages = mkIf cfg.pipewire.noise-cancelling.enable [
        pkgs.ladspaPlugins
        pkgs.deepfilternet
      ];
      extraConfig = {
        pipewire = {
          "95-fifine-am8-noise-cancelling" = mkIf cfg.pipewire.noise-cancelling.enable {
            "context.modules" = [
              {
                "name" = "libpipewire-module-filter-chain";
                "args" = {
                  "node.description" = "Noise Cancelled Voise (Fifine AM8)";
                  "media.name" = "Noise Cancelled Voice (Fifine AM8)";
                  "audio.rate" = 48000;
                  "filter.graph" = {
                    "nodes" = [
                      {
                        "type" = "ladspa";
                        "name" = "hpf";
                        "plugin" = "butterworth_1902";
                        "label" = "butthigh_iir";
                        "control" = {
                          "Cutoff Frequency (Hz)" = 120.0;
                          "Resonance" = 0.707;
                        };
                      }
                      {
                        "type" = "ladspa";
                        "name" = "gate";
                        "plugin" = "gate_1410";
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
                        plugin = "libdeep_filter_ladspa";
                        label = "deep_filter_mono";
                        control = {
                          "Attenuation Limit (dB)" = 100;
                        };
                      }
                      {
                        "type" = "ladspa";
                        "name" = "comp";
                        "plugin" = "sc4m_1916";
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
                      {
                        "type" = "ladspa";
                        "name" = "final_gate";
                        "plugin" = "gate_1410";
                        "label" = "gate";
                        "control" = {
                          "LF key filter (Hz)" = 100.0;
                          "HF key filter (Hz)" = 10000.0;
                          "Threshold (dB)" = -40.0;
                          "Attack (ms)" = 0.5;
                          "Hold (ms)" = 100.0;
                          "Decay (ms)" = 150.0;
                          "Range (dB)" = -90.0;
                          "Output select (-1 = key listen, 0 = gate, 1 = bypass)" = 0;
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
                      {
                        "output" = "comp:Output";
                        "input" = "final_gate:Input";
                      }
                    ];
                  };
                  "capture.props" = {
                    "node.name" = "fifine_am8_voice.input";
                    "node.passive" = true;
                    "node.latency" = "1024/48000";
                    "node.lock-rate" = true;
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                    "target.object" = cfg.pipewire.noise-cancelling.target;
                  };
                  "playback.props" = {
                    "node.name" = "fifine_am8_voice.source";
                    "media.class" = "Audio/Source";
                    "node.latency" = "1024/48000";
                    "node.lock-rate" = true;
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
  };
}
