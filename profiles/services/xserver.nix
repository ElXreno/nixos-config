{ lib, config, ... }: {
  services.xserver = {
    enable = true;
    layout = "us,ru";
    xkbOptions = "grp:alt_shift_toggle";
    libinput = lib.mkIf config.deviceSpecific.isLaptop {
      enable = true;
      mouse.accelSpeed = "-0.4";
      touchpad = {
        tappingDragLock = false;
        naturalScrolling = true;
      };
    };
  };
}
