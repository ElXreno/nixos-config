{ inputs, ... }:
_final: prev: {
  inherit (inputs.claude-code.packages.${prev.stdenv.hostPlatform.system}) claude-code;
}
