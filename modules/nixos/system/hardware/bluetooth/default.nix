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
    mkPackageOption
    mkMerge
    ;
  cfg = config.${namespace}.system.hardware.bluetooth;
in
{
  options.${namespace}.system.hardware.bluetooth = {
    enable = mkEnableOption "Whether or not to manage bluetooth.";
    xboxSupport = mkEnableOption "Whether to enable Xbox Controller support.";
    bluezPackage = mkPackageOption pkgs "bluez5-experimental" { };
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence.directories = [ "/var/lib/bluetooth" ];

    services.blueman.enable = true;

    hardware.bluetooth = {
      enable = true;
      package = cfg.bluezPackage;

      settings.General = mkMerge [
        {
          Experimental = true;
        }

        (mkIf cfg.xboxSupport {
          # https://www.reddit.com/r/NixOS/comments/1ch5d2p/comment/lkbabax/
          # for pairing bluetooth controller
          Privacy = "device";
          JustWorksRepairing = "always";
          Class = "0x000100";
          FastConnectable = true;
        })
      ];
    };

    boot.extraModprobeConfig = mkIf cfg.xboxSupport ''
      options bluetooth disable_ertm=Y
    '';

    services.pipewire.wireplumber.extraConfig = {
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
    };
  };
}
