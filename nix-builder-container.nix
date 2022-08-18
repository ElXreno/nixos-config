{ pkgs ? import <nixpkgs> { } }:

let
  sshd-config = pkgs.writeTextFile {
    name = "sshd-config";
    text = ''
      Port 2222
      PermitRootLogin yes
      PasswordAuthentication no
      AuthorizedKeysFile .ssh/authorized_keys
    '';
  };
in
pkgs.dockerTools.buildImage {
  name = "nix-builder";
  tag = "latest";
  runAsRoot = ''
    #!${pkgs.stdenv.shell}
    ${pkgs.dockerTools.shadowSetup}
    
  '';
  created = "now";
  contents = [
    pkgs.coreutils
    pkgs.nix
    pkgs.openssh
  ];
  config = {
    # Entrypoint = [ "${pkgs.openssh}/bin/sshd" "-D" "-e" ];
    Entrypoint = [ "${pkgs.bash}/bin/bash" ];
    Env = [
      "NIX_PAGER=cat"
      "USER=nobody"
    ];
  };
}
