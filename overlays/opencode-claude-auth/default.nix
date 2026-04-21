_:

_final: prev: {
  opencode-claude-auth = prev.opencode-claude-auth.overrideAttrs (
    _finalAttrs: _prevAttrs: {
      src = prev.fetchFromGitHub {
        owner = "Arkptz";
        repo = "opencode-claude-auth";
        rev = "853f21bc1f5f8b737ecdfa95e094418eaf4042ed";
        hash = "sha256-sBTwmYS2E2/5/dU9L9nYvHNkrYyrfy+0hG8AuUvlPF0=";
      };
    }
  );
}
