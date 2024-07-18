{ pkgs, ... }:
let
  phpMajorVer = toString 82;

  php = pkgs."php${phpMajorVer}".buildEnv {
    extensions = { enabled, all }: enabled ++ (with all; [ xdebug ]);
    extraConfig = ''
      xdebug.mode=debug
    '';
  };
in pkgs.mkShell {
  nativeBuildInputs = with pkgs; [ php jetbrains.phpstorm ];

  buildInputs = with pkgs; [ ];
}
