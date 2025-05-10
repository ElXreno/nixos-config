{ lib, config, ... }: {
  services = {
    xserver = {
      enable = true;
      xkb = {
        layout = "us,ru";
        options = "grp:alt_shift_toggle";
      };
    };

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
