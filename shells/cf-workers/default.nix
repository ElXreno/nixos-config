{ pkgs, ... }:
pkgs.mkShell {
  packages = [
    pkgs.nodejs
    pkgs.wrangler
  ];

  env.WRANGLER_SEND_METRICS = "false";
}
