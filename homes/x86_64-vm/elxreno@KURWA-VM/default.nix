{ lib, ... }:
{
  imports = [ (lib.snowfall.fs.get-file "homes/x86_64-linux/elxreno@KURWA/default.nix") ];
}
