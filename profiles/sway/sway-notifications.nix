{
  services.mako = {
    enable = true;
    anchor = "top-right";
    defaultTimeout = 5000;
    ignoreTimeout = true;
    padding = "10";
    font = "FiraCode Nerd Font 10";
  };
  wayland.windowManager.sway.config.startup = [
    { command = "mako"; }
  ];
}