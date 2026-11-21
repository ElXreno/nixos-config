{
  lib,
  namespace,
  nixosConfigurations,
}:

lib.filterAttrs (_: hostnames: hostnames != [ ]) (
  lib.mapAttrs' (
    name: machine:
    lib.nameValuePair (lib.toLower name) (
      lib.attrNames machine.config.${namespace}.services.cloudflared.ingress
    )
  ) nixosConfigurations
)
