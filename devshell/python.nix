{ pkgs, ... }:
let python = pkgs.python311;
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    pkg-config
    clang
    gnumake
    python3

    # For psycopg2
    postgresql
  ] ++ (with jetbrains; [
    pycharm-professional
  ]);

  buildInputs = with pkgs; with python.pkgs; [
    # Python
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
    PYTHONPATH=$venvDir/${python.sitePackages}:$PYTHONPATH
  '';
}
