{ namespace, pkgs, ... }:
{
  home.packages = [ pkgs.moonlight-qt ];

  programs.niri.settings.outputs = {
    "eDP-1" = {
      position = {
        x = 0;
        y = 360;
      };
      variable-refresh-rate = false;
    };
  };

  ${namespace} = {
    programs = {
      gpg.enable = true;

      # Integrated graphics is more power efficient
      # See `mpv --vulkan-device=help`
      mpv.vulkanDevice = "AMD Radeon 780M Graphics (RADV PHOENIX)";
    };

    services = {
      gpg-agent.enable = true;
    };
  };
}
