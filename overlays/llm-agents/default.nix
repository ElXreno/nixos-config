{ inputs, ... }:
_final: prev: {
  claude-code = inputs.llm-agents.packages.${prev.stdenv.hostPlatform.system}.claude-code.override {
    disableTelemetry = true;
  };
}
