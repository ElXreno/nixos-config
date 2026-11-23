_:

final: _prev: {
  ccacheTools = final.lib.makeOverridable (
    {
      pkgs ? final,
      cacheDir ? "/nix/ccache",
      settings ? {
        max_size = "300G";
        compression = true;
        umask = "000";
        sloppiness = "include_file_mtime,include_file_ctime,random_seed";
      },
    }:
    let
      inherit (pkgs.lib)
        isBool
        boolToString
        mapAttrsToList
        concatStringsSep
        ;

      renderSetting =
        key: value: "${key} = ${if isBool value then boolToString value else toString value}";

      confFile = pkgs.writeText "ccache.conf" (
        concatStringsSep "\n" (mapAttrsToList renderSetting settings) + "\n"
      );

      environment = compilerCheck: ''
        export CCACHE_DIR="${cacheDir}"
        export CCACHE_CONFIGPATH="${confFile}"
        export CCACHE_COMPILERCHECK="${compilerCheck}"
        if [ ! -d "$CCACHE_DIR" ] || [ ! -w "$CCACHE_DIR" ]; then
          export CCACHE_DISABLE=1
        fi
      '';
    in
    {
      inherit cacheDir settings confFile;

      wrapStdenv =
        stdenv:
        pkgs.stdenvAdapters.overrideCC stdenv (
          stdenv.cc.override {
            cc = pkgs.ccache.links {
              unwrappedCC = stdenv.cc.cc;
              extraConfig = environment "string:${stdenv.cc.cc}";
            };
          }
        );

      launcher = pkgs.writeShellScript "ccache-launcher" ''
        ${environment "content"}
        exec ${pkgs.lib.getExe pkgs.ccache} "$@"
      '';

      wrapCompiler =
        compiler: name:
        pkgs.writeShellScriptBin name ''
          ${environment "string:${compiler}"}
          exec ${pkgs.lib.getExe pkgs.ccache} ${pkgs.lib.getBin compiler}/bin/${name} "$@"
        '';
    }
  ) { };
}
