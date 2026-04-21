{ inputs, ... }:
_final: prev: {
  inherit (inputs.llm-agents.packages.${prev.stdenv.hostPlatform.system})
    claude-code
    ;
}
