{
  fetchFromGitHub,
  lib,
  rustPlatform,
}:

rustPlatform.buildRustPackage rec {
  pname = "crates-lsp";
  version = "0.4.2";

  src = fetchFromGitHub {
    owner = "MathiasPius";
    repo = pname;
    rev = "v${version}";
    hash = "sha256-s42nWQC2tD7vhQNPdTQNRokwXqeBhELidVYTlos+No0=";
  };

  cargoHash = "sha256-XqUWcbaOZXRWzIvL9Kbo6Unl0rmeGxHO4+674uHukAs=";

  meta = with lib; {
    description = "Language Server implementation for Cargo.toml";
    homepage = "https://github.com/MathiasPius/crates-lsp";
    mainProgram = pname;
    license = licenses.mit;
    platforms = platforms.linux;
  };
}
