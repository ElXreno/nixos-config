{ pkgs, ... }: {
  programs.rofi = {
    enable = true;
    package = pkgs.rofi-wayland;
    theme = "Arc-Dark";
    font = "FiraCode Nerd Font 14";
    plugins = with pkgs; [ rofi-calc ];
  };
}
