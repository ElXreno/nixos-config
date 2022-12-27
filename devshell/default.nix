{ pkgs, ... }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    # Rust
    pkg-config
    clang
    gnumake
    rustup
  ] ++ (with jetbrains; [
    clion
    idea-ultimate
  ]);

  buildInputs = with pkgs; [
    # Rust
    openssl
    sqlite
    gtk4
  ];
}
