{ pkgs, ... }:
{
  services.ananicy = {
    enable = true;
    package = pkgs.ananicy-cpp;
    rulesProvider = pkgs.ananicy-rules-cachyos;

    # ananicy-cpp tries to move some kernel stuff like `migration/0(24)`,
    # idle_inject/1(65), etc, so ignore some errors like `cgroup error: couldn't add task to cgroup`.
    # Details can be shown via:
    # settings = { loglevel = "debug"; };
  };

  # https://gitlab.com/ananicy-cpp/ananicy-cpp/-/issues/40#note_1036996573
  systemd.services."user@".serviceConfig.Delegate = "cpu cpuset io memory pids";
}
