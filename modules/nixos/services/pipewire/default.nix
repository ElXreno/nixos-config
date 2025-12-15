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
  cfg = config.${namespace}.services.pipewire;
in
{
  options.${namespace}.services.pipewire = {
    enable = mkEnableOption "Whether or not to manage pipewire.";
    rnnoise = {
      enable = mkEnableOption "Whether to enable RNNoise.";
      target = mkOption {
        type = with types; nullOr str;
        default = null;
      };
    };
    quantum = {
      min = mkOption {
        type = types.int;
        default = 512;
      };
      max = mkOption {
        type = types.int;
        default = 1024;
      };
    };
  };

  config = mkIf cfg.enable {
    assertions = optionals cfg.rnnoise.enable [
      {
        assertion = cfg.rnnoise.target != null;
        message = ''
          When `${namespace}.services.pipewire.rnnoise.enable` is set to true,
          `${namespace}.services.pipewire.rnnoise.target` must be set.
        '';
      }
    ];

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
            "default.clock.quantum" = cfg.quantum.min;
            "default.clock.min-quantum" = cfg.quantum.min;
            "default.clock.max-quantum" = cfg.quantum.max;
          };
          "80-allowed-rates"."context.properties" = {
            "default.clock.rate" = 48000;
            "default.clock.allowed-rates" = [
              48000
              96000
              192000
            ];
          };
          "95-generic-noise-cancelling" = mkIf cfg.rnnoise.enable {
            "context.modules" = [
              {
                "name" = "libpipewire-module-filter-chain";
                "args" = {
                  "node.description" = "Noise Cancelled Voise (Generic)";
                  "media.name" = "Noise Cancelled Voise (Generic)";
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
                      {
                        "type" = "ladspa";
                        "name" = "comp";
                        "plugin" = "${pkgs.ladspaPlugins}/lib/ladspa/sc4m_1916.so";
                        "label" = "sc4m";
                        "control" = {
                          "Attack time (ms)" = 20.0;
                          "Release time (ms)" = 500.0;
                          "Threshold level (dB)" = -18.0;
                          "Ratio (1:n)" = 4.0;
                          "Knee radius (dB)" = 3.0;
                          "Makeup gain (dB)" = 6.0;
                        };
                      }
                    ];
                    "links" = [
                      {
                        "output" = "rnnoise:Output";
                        "input" = "comp:Input";
                      }
                    ];
                  };
                  "capture.props" = {
                    "node.name" = "generic_rnnoise_voice.input";
                    "node.passive" = true;
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                    "target.object" = cfg.rnnoise.target;
                  };
                  "playback.props" = {
                    "node.name" = "generic_rnnoise_voice.source";
                    "media.class" = "Audio/Source";
                    "audio.rate" = 48000;
                    "audio.channels" = 1;
                    "audio.position" = [ "MONO" ];
                  };
                };
              }
            ];
          };
          "99-force-roles" = {
            "pulse.rules" = [
              {
                "matches" = [
                  { "application.process.binary" = ".Discord-wrapper"; }
                  { "application.process.binary" = ".Telegram-wrapped"; }
                ];
                "actions" = {
                  "update-props" = {
                    "media.role" = "phone";
                  };
                };
              }
              {
                "matches" = [
                  { "application.process.binary" = "firefox"; }
                  { "application.process.binary" = "spotify"; }
                ];
                "actions" = {
                  "update-props" = {
                    "media.role" = "music";
                  };
                };
              }
            ];
          };
        };
      };
      wireplumber.extraConfig."50-role-ducking" = {
        "wireplumber.profiles" = {
          main = {
            "policy.linking.role-based.loopbacks" = "required";
            "loopback.sink.role.multimedia" = "required";
            "loopback.sink.role.voice" = "required";
          };
        };

        "wireplumber.settings" = {
          "node.stream.default-media-role" = "Multimedia";
          "linking.role-based.duck-level" = 0.3;
        };

        "wireplumber.components" =
          let
            mkLoopbackSink =
              {
                name,
                description,
                intendedRoles,
                priority,
                lowerPriorityAction,
              }:
              {
                type = "pw-module";
                name = "libpipewire-module-loopback";
                arguments = {
                  "node.name" = "loopback.sink.role.${name}";
                  "node.description" = description;
                  "audio.position" = [
                    "FL"
                    "FR"
                  ];
                  "capture.props" = {
                    "device.intended-roles" = intendedRoles;
                    "policy.role-based.priority" = priority;
                    "policy.role-based.action.same-priority" = "mix";
                    "policy.role-based.action.lower-priority" = lowerPriorityAction;
                    "policy.role-based.target" = true;
                    "media.class" = "Audio/Sink";
                  };
                  "playback.props" = {
                    "node.passive" = true;
                    "media.role" = "Loopback";
                  };
                };
                provides = "loopback.sink.role.${name}";
              };
          in
          [
            (mkLoopbackSink {
              name = "multimedia";
              description = "Multimedia (Ducked)";
              intendedRoles = [
                "Music"
                "Movie"
                "Game"
                "Multimedia"
              ];
              priority = 10;
              lowerPriorityAction = "mix";
            })

            (mkLoopbackSink {
              name = "voice";
              description = "Voice Communication";
              intendedRoles = [
                "phone"
                "Communication"
                "Assistant"
                "Voice"
              ];
              priority = 50;
              lowerPriorityAction = "duck";
            })

            {
              type = "virtual";
              provides = "policy.linking.role-based.loopbacks";
              requires = [ ];
            }
          ];
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
