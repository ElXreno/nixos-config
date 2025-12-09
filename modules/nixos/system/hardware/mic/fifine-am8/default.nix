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
    optionals
    ;
  cfg = config.${namespace}.system.hardware.fifine-am8;
in
{
  options.${namespace}.system.hardware.fifine-am8 = {
    enable = mkEnableOption "Whether or not to manage Fifine AM8 stuff.";

    vendorId = mkOption {
      type = types.str;
      default = "3142";
    };
    productId = mkOption {
      type = types.str;
      default = "a010";
    };

    disableSidetone = mkEnableOption "Whether to disable sidetone by default" // {
      default = true;
    };

    pipewire.noise-cancelling = {
      enable = mkEnableOption "Whether to enable noise cancelling" // {
        default = true;
      };

      target = mkOption {
        type = with types; nullOr str;
        default = null;
      };
    };
  };

  config = mkIf cfg.enable {
    assertions = optionals cfg.pipewire.noise-cancelling.enable [
      {
        assertion = cfg.pipewire.noise-cancelling.target != null;
        message = ''
          When `${namespace}.system.hardware.fifine-am8.pipewire.noise-cancelling.enable` is set to true,
          `${namespace}.system.hardware.fifine-am8.pipewire.noise-cancelling.target` must be set.
        '';
      }
    ];

    ${namespace}.services.pipewire = {
      # deepfilternet is heavy for CPU
      quantum = {
        min = 1024;
        max = 4096;
      };
    };

    services.udev.extraRules = mkIf cfg.disableSidetone ''
      ACTION=="add|change", SUBSYSTEM=="sound", KERNEL=="controlC*", ATTRS{idVendor}=="${cfg.vendorId}", ATTRS{idProduct}=="${cfg.productId}", RUN+="${pkgs.alsa-utils}/bin/amixer -c %n cset name='Mic Playback Switch' off"
    '';

    services.pipewire = {
      extraConfig = {
        pipewire = {
          "95-fifine-am8-noise-cancelling" = mkIf cfg.pipewire.noise-cancelling.enable {
            "context.modules" = [
              {
                "name" = "libpipewire-module-filter-chain";
                "args" = {
                  "node.description" = "Noise Cancelled Voise (Fifine AM8)";
                  "media.name" = "Noise Cancelled Voice (Fifine AM8)";
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
                      {
                        "type" = "ladspa";
                        "name" = "final_gate";
                        "plugin" = "${pkgs.ladspaPlugins}/lib/ladspa/gate_1410.so";
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
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                    "target.object" = cfg.pipewire.noise-cancelling.target;
                  };
                  "playback.props" = {
                    "node.name" = "fifine_am8_voice.source";
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
  };
}
