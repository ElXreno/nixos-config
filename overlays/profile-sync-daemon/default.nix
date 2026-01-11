_:

_final: prev: {
  profile-sync-daemon = prev.profile-sync-daemon.overrideAttrs (
    _finalAttrs: _prevAttrs: {
      version = "unstable-2025-07-25";

      src = prev.fetchFromGitHub {
        owner = "graysky2";
        repo = "profile-sync-daemon";
        rev = "bdea6f002018bd23e8734e87974553835b1d45e9";
        hash = "sha256-GIj2AjTWo9M3k/5R3FnYQBpHvXiIrF+1gPVHUGb6mp4=";
      };
    }
  );
}
