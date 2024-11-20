{ pkgs, ... }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs;
    [
      # Rust
      pkg-config
      clang
      gnumake
      rustup
    ] ++ (with jetbrains; [ rust-rover ]);

  buildInputs = with pkgs; [
    # Rust
    openssl
    sqlite
    gtk4
  ];

  NIX_LD = pkgs.lib.fileContents "${pkgs.stdenv.cc}/nix-support/dynamic-linker";
}
