{ pkgs, ... }:
let python = pkgs.python39;
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    pkg-config
    clang
    gnumake
    rustup
    python3

    # bruh
    php
  ] ++ (with jetbrains; with androidStudioPackages; [
    clion
    idea-ultimate
    phpstorm
    rider

    canary
  ]);

  buildInputs = with pkgs; with python.pkgs; [
    # Rust
    openssl
    sqlite
    gtk4

    # C#
    dotnet-sdk_6
    mono6

    # Python
    setuptools
    wheel
    venvShellHook
  ];

  venvDir = "/home/elxreno/.cache/nix-python-venv";

  postVenv = ''
    unset SOURCE_DATE_EPOCH
    ./scripts/install_local_packages.sh
  '';
  postShellHook = ''
    # Allow the use of wheels.
    unset SOURCE_DATE_EPOCH
    PYTHONPATH=$venvDir/${python.sitePackages}:$PYTHONPATH
  '';
}
