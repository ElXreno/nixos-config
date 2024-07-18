{ pkgs, ... }:
let
  lock = pkgs.writeScript "lock" ''
    ${pkgs.swaylock-effects}/bin/swaylock -f --screenshots --clock --effect-greyscale
  '';
  unlock = pkgs.writeScript "unlock" ''
    ${pkgs.procps}/bin/pkill swaylock
  '';
  screen-off = pkgs.writeScript "screenOff" ''
    ${pkgs.sway}/bin/swaymsg "output * dpms off"
  '';
  resume = pkgs.writeScript "resume" ''
    ${pkgs.sway}/bin/swaymsg "output * dpms on"
  '';
in {
  services.swayidle = {
    enable = true;
    events = [
      {
        event = "before-sleep";
        command = "${lock}";
      }
      {
        event = "lock";
        command = "${lock}";
      }
      {
        event = "unlock";
        command = "${unlock}";
      }
    ];
    timeouts = [
      {
        timeout = 600;
        command = "${screen-off}";
        resumeCommand = "${resume}";
      }
      {
        timeout = 610;
        command = "${lock}";
        resumeCommand = "${resume}";
      }
    ];
  };
}
