{
  lib,
  buildNpmPackage,
  fetchFromGitHub,
}:
buildNpmPackage (finalAttrs: {
  pname = "opencode-dcp";
  version = "3.1.12";

  src = fetchFromGitHub {
    owner = "Opencode-DCP";
    repo = "opencode-dynamic-context-pruning";
    tag = "v${finalAttrs.version}";
    hash = "sha256-dtLb7fU990LvdSv5em1OkiyUgSvr1Tgt+WqHNw2mRPA=";
  };

  prePatch = ''
    substituteInPlace tsup.config.ts \
      --replace-fail 'noExternal: ["jsonc-parser"]' 'noExternal: ["jsonc-parser", "@opencode-ai/plugin"]'
  '';

  npmDepsHash = "sha256-goPt3G7XbrH73y0RZezAg9IsxacywrBIbf6jVvAfAkM=";

  passthru.pluginDir = "${finalAttrs.finalPackage}/lib/node_modules/@tarquinen/opencode-dcp";

  meta = {
    description = "OpenCode plugin that optimizes token usage by pruning obsolete tool outputs from conversation context";
    homepage = "https://github.com/Opencode-DCP/opencode-dynamic-context-pruning";
    license = lib.licenses.agpl3Plus;
    maintainers = with lib.maintainers; [ elxreno ];
    platforms = lib.platforms.all;
  };
})
