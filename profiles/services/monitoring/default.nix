{ ... }:
{
  imports = [
    ./exporters.nix
    ./grafana.nix
    ./victorialogs.nix
    ./victoriametrics.nix
  ];
}
