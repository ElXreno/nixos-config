{ config, ... }: {
  home-manager.users.elxreno.programs.git = {
    enable = true;
    lfs.enable = true;
    aliases = { cp = "cherry-pick"; };
    signing = {
      key = "C573235A0F2B0FE2";
      signByDefault = !config.deviceSpecific.isServer;
    };
    userEmail = "elxreno@gmail.com";
    userName = "ElXreno";
    extraConfig = {
      pull = { rebase = true; };
      init = {
        defaultBranch = "main";
        # templateDir = lib.mkIf (!config.deviceSpecific.isServer) "~/.git-templates";
      };
      fetch = { prune = true; };
      core = { autocrlf = "input"; };
      gc.auto = 0;
    };
  };
}
