{ pkgs, ... }:
let
  myPython = pkgs.python311;
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    pkg-config
    clang
    gnumake
    myPython
  ];

  buildInputs = with myPython.pkgs; [
    venvShellHook
  ];

  NIX_LD = pkgs.lib.fileContents "${pkgs.stdenv.cc}/nix-support/dynamic-linker";

  venvDir = "./venv";

  postVenvCreation = ''
    unset SOURCE_DATE_EPOCH
    pip install -r requirements.txt
  '';

  postShellHook = ''
    # Allow the use of wheels.
    unset SOURCE_DATE_EPOCH
    PYTHONPATH=$venvDir/${myPython.sitePackages}:$PYTHONPATH
  '';
}
