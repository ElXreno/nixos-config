{ pkgs, namespace, ... }:
let
  guard = pkgs.${namespace}.agent-guard;
in
pkgs.mkShell {
  inputsFrom = [ guard ];
  env = guard.testEnv;
}
