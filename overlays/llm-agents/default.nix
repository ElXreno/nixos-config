{ inputs, ... }:
let
  llm-agents = prev: inputs.llm-agents.packages.${prev.stdenv.hostPlatform.system};
  bunpatch =
    prev:
    prev.buildGoModule {
      pname = "bunpatch";
      version = "0";
      src = ./bunpatch;
      vendorHash = null;
      meta.mainProgram = "bunpatch";
    };
in
_final: prev: {
  inherit (llm-agents prev)
    agent-browser
    herdr
    ;
  claude-code =
    ((llm-agents prev).claude-code.override {
      disableTelemetry = true;
    }).overrideAttrs
      (old: {
        postInstall = (old.postInstall or "") + ''
          ${prev.lib.getExe (bunpatch prev)} $out/bin/claude ${./claude-code-live-progress.json} ${./claude-code-ask-timeout.json}
        '';
        postFixup = old.postFixup + ''
          wrapProgram $out/bin/claude \
            --argv0 claude \
            --prefix PATH : ${prev.lib.makeBinPath [ prev.nodejs ]} \
            --run 'if [ "''${TERM_PROGRAM-}" = herdr ]; then export TERM_PROGRAM=kitty; fi'
        '';
      });
}
