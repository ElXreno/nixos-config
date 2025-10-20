_:

_final: prev: {
  ollama-rocm-igpu = prev.ollama.overrideAttrs (
    _finalAttrs: prevAttrs: {
      src = prev.fetchFromGitHub {
        owner = "ollama";
        repo = "ollama";
        rev = "7ed0c4d9cce7dd4d729ef724fcd9839259c57809"; # https://github.com/ollama/ollama/pull/6282
        hash = "sha256-FU2XY4kInP0a0sQDfqmLWKOPhE8QfsAK98PpqRM70IY=";
      };

      vendorHash = "sha256-4wYgtdCHvz+ENNMiHptu6ulPJAznkWetQcdba3IEB6s=";
    }
  );
}
