{
  pkgs,
  config,
  lib,
  ...
}:
let
  hasBluetooth = config.deviceSpecific.isLaptop;
in
{
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    alsa.support32Bit = true;
    pulse.enable = true;

    wireplumber.extraConfig = lib.mkMerge [
      (lib.mkIf hasBluetooth {
        "10-bluez" = {
          "monitor.bluez.rules" = [
            {
              matches = [ { "device.name" = "~bluez_card.*"; } ];
              actions = {
                update-props = {
                  "bluez5.roles" = [
                    "hsp_hs"
                    "hsp_ag"
                    "hfp_hf"
                    "hfp_ag"
                  ];
                  "bluez5.enable-msbc" = true;
                  "bluez5.enable-sbc-xq" = true;
                  "bluez5.enable-hw-volume" = true;
                  "bluez5.a2dp.ldac.quality" = "hq";
                };
              };
            }
          ];
        };
      })
    ];

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
      };
    };
  };

  services.udev.extraHwdb = lib.mkIf (config.device == "INFINITY") ''
    evdev:name:Huawei WMI hotkeys:*
      KEYBOARD_KEY_287=f20
  '';

  boot.extraModprobeConfig = lib.mkIf hasBluetooth ''
    options bluetooth disable_ertm=Y
  '';

  hardware = {
    cpu = {
      amd.updateMicrocode = lib.mkIf (
        config.device == "AMD-Desktop" || config.device == "INFINITY" || config.device == "KURWA"
      ) true;
    };

    graphics = {
      enable = true;
      enable32Bit = true;
    };

    amdgpu.amdvlk = lib.mkIf (config.device == "INFINITY") {
      enable = true;
      support32Bit.enable = true;
    };

    bluetooth = {
      # TODO: Fix state after persist
      enable = hasBluetooth;
      # For battery provider, bluezFull is just an alias for bluez
      package = pkgs.bluez5-experimental;

      settings.General = {
        Experimental = true;
        # https://www.reddit.com/r/NixOS/comments/1ch5d2p/comment/lkbabax/
        # for pairing bluetooth controller
        Privacy = "device";
        JustWorksRepairing = "always";
        Class = "0x000100";
        FastConnectable = true;
      };
    };
  };

  # Disable suspend on lid switch
  services.logind.settings.Login.HandleLidSwitch = "ignore";

  systemd.services.NetworkManager-wait-online.enable = false;
}
