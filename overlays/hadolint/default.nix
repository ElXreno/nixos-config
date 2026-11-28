_: _final: prev:
let
  hl = prev.haskell.lib.compose;

  haskellPackages = prev.haskellPackages.override (old: {
    overrides = prev.lib.composeExtensions (old.overrides or (_: _: { })) (
      _: super: {
        spdx = hl.doJailbreak super.spdx;
        language-docker = hl.doJailbreak super.language-docker;
      }
    );
  });
in
{
  hadolint = hl.justStaticExecutables haskellPackages.hadolint;
}
