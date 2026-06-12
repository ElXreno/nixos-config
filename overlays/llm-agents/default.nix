{ inputs, ... }:
let
  llm-agents = prev: inputs.llm-agents.packages.${prev.stdenv.hostPlatform.system};
in
_final: prev: {
  inherit (llm-agents prev) codex opencode;
  claude-code =
    ((llm-agents prev).claude-code.override {
      disableTelemetry = true;
    }).overrideAttrs
      (old: {
        postFixup = old.postFixup + ''
          wrapProgram $out/bin/claude \
            --argv0 claude \
            --prefix PATH : ${prev.lib.makeBinPath [ prev.nodejs ]}
        '';
      });
}
