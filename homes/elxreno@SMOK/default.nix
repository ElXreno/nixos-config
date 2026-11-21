{ namespace, pkgs, ... }:
{
  home.packages = with pkgs; [
    qbittorrent

    # Games
    taterclient-ddnet
    prismlauncher
    xclicker
    bottles

    # MEGA
    megasync

    # Libvirtd manager
    virt-manager
  ];

  programs.niri.settings.outputs = {
    "eDP-1" = {
      mode = {
        width = 3840;
        height = 2400;
        refresh = 240.0;
      };
      position = {
        x = 0;
        y = 360;
      };
      variable-refresh-rate = true;
      srgb-clamp = true;
      srgb-clamp-correction = ../../machines/SMOK/display-calibration/BOE3840_sRGB_6500.txt;
    };
    "Xiaomi Corporation Mi monitor 5392700044842" = {
      mode = {
        width = 2560;
        height = 1440;
        refresh = 180.0;
      };
      focus-at-startup = true;
      variable-refresh-rate = true;
      max-bpc = 8;
      dithering = "static-2x2";
      srgb-clamp = true;
    };
  };

  ${namespace} = {
    programs = {
      memory = {
        enable = true;
        disableBuiltin = true;
        embed.url = "http://kurwa:8096,http://bimba:8096";
        rerank.url = "http://kurwa:8101,http://bimba:8101";
        recall = {
          pool = "reg";
          extraArgs = [
            "--reg-cos"
            "0.65"
            "--reg-margin"
            "0.05"
            "--reg-pointer-cos"
            "0.5"
            "--embed-max-chars"
            "2000"
            "--rerank-doc-chars"
            "2000"
            "--rerank-timeout"
            "6s"
          ];
        };
        shadowPool = "fused-reg";
      };
      gpg.enable = true;
    };

    services = {
      gpg-agent.enable = true;
    };
  };
}
