_:

_final: prev: {
  sing-box = prev.sing-box.overrideAttrs (
    _finalAttrs: prevAttrs: {
      patches = (prevAttrs.patches or [ ]) ++ [
        ./ebpf-process-searcher.patch
      ];

      nativeBuildInputs = (prevAttrs.nativeBuildInputs or [ ]) ++ [
        prev.llvmPackages.clang-unwrapped
        prev.libbpf
        prev.linuxHeaders
      ];

      # Run go generate to compile eBPF C → .o before go build
      preBuild = (prevAttrs.preBuild or "") + ''
        export C_INCLUDE_PATH="${prev.libbpf}/include:${prev.linuxHeaders}/include"
        go generate ./common/process/...
      '';

      # Clear preBuild in the goModules derivation — go generate must not
      # run there because vendor/ doesn't exist yet at that point.
      passthru = (prevAttrs.passthru or { }) // {
        overrideModAttrs = _finalModAttrs: _prevModAttrs: {
          preBuild = "";
        };
      };

      # vendorHash changes because go.mod adds cilium/ebpf
      vendorHash = "sha256-/NCbiC5pMTFPy0POBT3Xz91I/+F1y+R0XKh4jArdiJ4=";
    }
  );
}
