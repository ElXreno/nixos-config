{ pkgs, ... }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    pkg-config
    clang
    gnumake
    rustup

    # bruh
    php
  ] ++ (with jetbrains; [
    clion
    idea-ultimate
    phpstorm
  ]);

  buildInputs = with pkgs; [
    # Rust
    openssl
    sqlite
    gtk4
  ];
}
