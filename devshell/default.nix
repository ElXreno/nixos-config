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
  ];

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

  venvDir = "/home/elxreno/projects/repos/github.com/ElXreno/yt-dlp/venv";

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
