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
  ] ++ (with jetbrains; [
    pycharm-professional
  ]);

  buildInputs = with pkgs; with python.pkgs; [
    # Python
    aiohttp
    setuptools
    toml
    wheel
    venvShellHook
  ];

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
