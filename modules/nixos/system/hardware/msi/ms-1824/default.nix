{
  config,
  inputs,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    concatLists
    imap0
    mkIf
    mkEnableOption
    tail
    zipListsWith
    ;
  cfg = config.${namespace}.system.hardware.msi.ms-1824;

  msi-ec = config.boot.kernelPackages.msi-ec.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [
      (pkgs.fetchpatch {
        url = "https://github.com/sidneib-accurati/msi-ec/commit/972f5a5f0c4ce945b21eec6f97f7935fb32aab93.patch";
        hash = "sha256-YRAFRRnZsBDay/55GwDxKvc5/vZ+sAzRw1Sxb+ejoWQ=";
      })
    ];
  });

  speakerFir = "${pkgs.${namespace}.msi-1824-speaker-fir}/share/msi-1824/speakers-fir.wav";
  codec = "pci-0000_80_1f.3-platform-skl_hda_dsp_generic.HiFi";
  speakerSink = "alsa_output.${codec}__Speaker__sink";

  speakerChannels = [
    "l"
    "r"
  ];

  speakerBiquad = label: name: control: {
    type = "builtin";
    inherit name label control;
  };

  speakerPlain = label: name: {
    type = "builtin";
    inherit name label;
  };

  speakerButterworth =
    label: name: freq:
    speakerBiquad label name {
      Freq = freq;
      Q = 0.70710677;
    };

  speakerLimiter = name: limit: {
    type = "ladspa";
    inherit name;
    plugin = "fast_lookahead_limiter_1913";
    label = "fastLookaheadLimiter";
    control = {
      "Input gain (dB)" = 0.0;
      "Limit (dB)" = limit;
      "Release time (s)" = 0.1;
    };
  };

  speakerLink = output: input: { inherit output input; };

  speakerChain = names: zipListsWith (a: b: speakerLink "${a}:Out" "${b}:In") names (tail names);

  speakerCrossover = {
    Freq = 400.0;
    Q = 0.70710677;
  };

  speakerGraph = {
    nodes =
      concatLists (
        imap0 (i: c: [
          (speakerPlain "copy" "in_${c}")
          (speakerBiquad "mixer" "add_${c}" {
            "Gain 1" = 1.0;
            "Gain 2" = 2.0;
          })
          (speakerBiquad "bq_peaking" "bass_${c}" {
            Freq = 200.0;
            Q = 2.020075;
            Gain = 6.0;
          })
          (speakerBiquad "bq_peaking" "treble_${c}" {
            Freq = 8000.0;
            Q = 1.663112;
            Gain = 4.0;
          })
          {
            type = "builtin";
            name = "fir_${c}";
            label = "convolver";
            config = {
              filename = speakerFir;
              channel = i;
              gain = 0.7586;
            };
          }
          (speakerBiquad "bq_lowpass" "lp1_${c}" speakerCrossover)
          (speakerBiquad "bq_lowpass" "lp2_${c}" speakerCrossover)
          (speakerBiquad "bq_highpass" "hp1_${c}" speakerCrossover)
          (speakerBiquad "bq_highpass" "hp2_${c}" speakerCrossover)
          {
            type = "builtin";
            name = "delay_${c}";
            label = "delay";
            config."max-delay" = 0.1;
            control."Delay (s)" = 0.005;
          }
          {
            type = "builtin";
            name = "mix_${c}";
            label = "mixer";
          }
        ]) speakerChannels
      )
      ++ [
        (speakerBiquad "mixer" "vbass_sum" {
          "Gain 1" = 0.5;
          "Gain 2" = 0.5;
        })
        (speakerButterworth "bq_lowpass" "vbass_lp1" 120.0)
        (speakerButterworth "bq_lowpass" "vbass_lp2" 120.0)
        (speakerButterworth "bq_highpass" "vbass_band" 45.0)
        (speakerPlain "abs" "vbass_even")
        (speakerButterworth "bq_highpass" "vbass_dc" 20.0)
        (speakerBiquad "mixer" "vbass_mix" {
          "Gain 1" = 1.0;
          "Gain 2" = 1.0;
        })
        (speakerPlain "abs" "vbass_odd")
        (speakerButterworth "bq_highpass" "vbass_hp1" 100.0)
        (speakerButterworth "bq_highpass" "vbass_hp2" 100.0)
        (speakerButterworth "bq_lowpass" "vbass_lp3" 320.0)
        (speakerButterworth "bq_lowpass" "vbass_out" 320.0)
        (speakerLimiter "limiter_low" 0.0)
        (speakerLimiter "limiter" (-0.1))
      ];
    links =
      speakerChain [
        "vbass_sum"
        "vbass_lp1"
        "vbass_lp2"
        "vbass_band"
        "vbass_even"
        "vbass_dc"
      ]
      ++ [
        (speakerLink "vbass_band:Out" "vbass_mix:In 1")
        (speakerLink "vbass_dc:Out" "vbass_mix:In 2")
      ]
      ++ speakerChain [
        "vbass_mix"
        "vbass_odd"
        "vbass_hp1"
        "vbass_hp2"
        "vbass_lp3"
        "vbass_out"
      ]
      ++ concatLists (
        imap0 (
          i: c:
          let
            port = toString (i + 1);
          in
          [
            (speakerLink "in_${c}:Out" "vbass_sum:In ${port}")
            (speakerLink "in_${c}:Out" "add_${c}:In 1")
            (speakerLink "vbass_out:Out" "add_${c}:In 2")
            (speakerLink "add_${c}:Out" "bass_${c}:In")
            (speakerLink "bass_${c}:Out" "treble_${c}:In")
            (speakerLink "treble_${c}:Out" "fir_${c}:In")
            (speakerLink "fir_${c}:Out" "lp1_${c}:In")
            (speakerLink "lp1_${c}:Out" "lp2_${c}:In")
            (speakerLink "lp2_${c}:Out" "limiter_low:Input ${port}")
            (speakerLink "limiter_low:Output ${port}" "mix_${c}:In 1")
            (speakerLink "fir_${c}:Out" "hp1_${c}:In")
            (speakerLink "hp1_${c}:Out" "hp2_${c}:In")
            (speakerLink "hp2_${c}:Out" "delay_${c}:In")
            (speakerLink "delay_${c}:Out" "mix_${c}:In 2")
            (speakerLink "mix_${c}:Out" "limiter:Input ${port}")
          ]
        ) speakerChannels
      );
    inputs = map (c: "in_${c}:In") speakerChannels;
    outputs = [
      "limiter:Output 1"
      "limiter:Output 2"
    ];
  };
in
{
  imports = [ inputs.ryukaze.nixosModules.default ];

  options.${namespace}.system.hardware.msi.ms-1824 = {
    enable = mkEnableOption "managing MSI MS-1824 stuff" // {
      default = config.${namespace}.facts.system.board == "MS-1824";
      defaultText = "Whether the SMBIOS board is MS-1824.";
    };
    speakers.enable = mkEnableOption "the factory speaker correction and limiter" // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      services.pipewire.rnnoise.target = "alsa_input.${codec}__Mic1__source";
      system = {
        hardware.gpu.nvidia.dynamicBoost.enable = true;
        impermanence.directories = [ "/var/lib/ryukaze" ];
      };
    };

    boot = {
      extraModulePackages = [ msi-ec ];
      kernelModules = [ "msi-ec" ];
      kernelParams = [
        "i915.enable_dpcd_backlight=3"
        "pcie_aspm.policy=powersupersave"
        "reboot=pci"
      ];
      plymouth.extraConfig = "DeviceScale=1";
    };

    fonts.fontconfig.subpixel.rgba = "rgb";

    services.udev.extraRules = ''
      ACTION=="add", SUBSYSTEM=="i2c", ATTR{name}=="SMBus I801 adapter at *", ATTR{new_device}="spd5118 0x53"
    '';

    services.flydigictl.settings.curves = mkIf config.services.flydigictl.enable [
      {
        name = "case";
        sensor = {
          hwmon = "thermal";
          label = "SEN3";
        };
        panic_c = 60;
        points = [
          {
            temp_c = 40;
            rpm = 0;
          }
          {
            temp_c = 41;
            rpm = 600;
          }
          {
            temp_c = 45;
            rpm = 1100;
          }
          {
            temp_c = 49;
            rpm = 1800;
          }
          {
            temp_c = 53;
            rpm = 2800;
          }
          {
            temp_c = 57;
            rpm = 4000;
          }
        ];
      }
    ];

    services.ryukaze = {
      enable = true;
      users = [ config.${namespace}.primaryUser ];
      tray.enable = true;
      osd.enable = true;
      settings = {
        apex_on_ac = true;
        charge_limit = 70;
        cooler_boost = {
          auto = true;
          cpu_c = 100;
          gpu_c = 82;
        };
        fans = {
          gpu_follows_cpu = true;
          cpu = {
            temps = [
              61
              67
              73
              79
              85
              90
            ];
            speeds = [
              20
              20
              35
              44
              58
              75
              100
            ];
          };
          gpu = {
            temps = [
              52
              58
              64
              70
              76
              82
            ];
            speeds = [
              20
              20
              35
              44
              58
              75
              100
            ];
          };
        };
        keyboard = {
          profile = "off";
          profiles = [
            {
              name = "off";
              color = "off";
            }
          ];
        };
        profiles = {
          ac = {
            scenario = "performance";
            fan_mode = "advanced";
          };
          battery.fan_mode = "advanced";
        };
        scenarios = [
          {
            name = "silent";
            shift_mode = "eco";
            fan_mode = "advanced";
            super_battery = false;
          }
          {
            name = "balanced";
            shift_mode = "comfort";
            fan_mode = "advanced";
            super_battery = false;
          }
          {
            name = "performance";
            shift_mode = "apex";
            fan_mode = "advanced";
            super_battery = false;
          }
        ];
      };
    };

    services.pipewire = mkIf cfg.speakers.enable {
      extraLadspaPackages = [ pkgs.ladspaPlugins ];
      wireplumber.extraConfig."60-ms-1824-speakers"."monitor.alsa.rules" = [
        {
          matches = [ { "node.name" = speakerSink; } ];
          actions.update-props = {
            "node.description" = "Titan Speakers";
            "audioconvert.filter-graph.0" = speakerGraph;
          };
        }
      ];
    };
  };
}
