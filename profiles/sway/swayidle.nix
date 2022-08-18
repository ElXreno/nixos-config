{ pkgs, lock, unlock, screen-off, resume, ... }:

{
  home-manager.users.elxreno.services.swayidle = {
    enable = true;
    events = [
      { event = "before-sleep"; command = "${lock}"; }
      { event = "lock"; command = "${lock}"; }
      { event = "unlock"; command = "${unlock}"; }
    ];
    timeouts = [
      { timeout = 600; command = "${screen-off}"; resumeCommand = "${resume}"; }
      { timeout = 610; command = "${lock}"; resumeCommand = "${resume}"; }
    ];
  };
}
