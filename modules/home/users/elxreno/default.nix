{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    getExe
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.users.elxreno;
  inherit (config.${namespace}) roles;
  sshKeys = import ../../../../inventory/_ssh-keys.nix;
  gitSignSk = pkgs.writeShellApplication {
    name = "git-sign-sk";
    runtimeInputs = with pkgs; [
      coreutils
      findutils
      gawk
      openssh
    ];
    text = builtins.readFile ./git-sign-sk.sh;
  };
in
{
  options.${namespace}.users.elxreno = {
    enable = mkEnableOption "configuring ElXreno user" // {
      default = config.home.username == "elxreno";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.programs.firefox = {
      extensions.packages = with pkgs.firefox-addons; [ keepassxc-browser ];
    };

    programs.git = {
      settings.user = {
        email = "elxreno@gmail.com";
        name = "ElXreno";
      };

      settings.sendemail = {
        smtpServer = "smtp.gmail.com";
        smtpServerPort = 587;
        smtpEncryption = "tls";
        smtpUser = config.programs.git.settings.user.email;
      };

      signing = {
        format = "ssh";
        key = "~/.ssh/id_ed25519_sk.pub";
        signByDefault = !roles.server.enable;
        signer = getExe gitSignSk;
        allowedSigners = "${config.programs.git.settings.user.email} ${sshKeys.yubikey}";
      };
    };
  };
}
