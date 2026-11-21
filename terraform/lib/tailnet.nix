{
  lib,
  namespace,
  nixosConfigurations,
}:

let
  sortStrings = list: lib.unique (builtins.sort builtins.lessThan list);

  machines = lib.filterAttrs (
    _: machine: machine.config.services.tailscale.enable
  ) nixosConfigurations;

  declarations = lib.concatMap (
    machine:
    lib.mapAttrsToList (svcName: svc: {
      name = lib.removePrefix "svc:" svcName;
      inherit (svc) comment tags;
      ports = lib.attrNames svc.endpoints;
    }) machine.config.${namespace}.tailscale.serve
  ) (lib.attrValues machines);

  services = lib.mapAttrs (_: hosts: {
    comment = lib.findFirst (comment: comment != "") "" (map (host: host.comment) hosts);
    tags = sortStrings (lib.concatMap (host: host.tags) hosts);
    ports = sortStrings (lib.concatMap (host: host.ports) hosts);
  }) (lib.groupBy (declaration: declaration.name) declarations);

  sharedPorts = lib.concatMap (
    machine:
    let
      shared = lib.filterAttrs (_: entry: entry.shared) machine.config.${namespace}.tailscale.serveNode;
    in
    lib.optional (shared != { }) {
      tags = machine.config.${namespace}.tailscale.deviceTags;
      ports = lib.attrNames shared;
    }
  ) (lib.attrValues machines);

  devices =
    lib.mapAttrs (_: machine: { tags = machine.config.${namespace}.tailscale.deviceTags; }) machines
    // {
      STARY.tags = [ "tag:dmz" ];
    };
  trustedTags = sortStrings (
    lib.concatMap (machine: machine.config.${namespace}.tailscale.deviceTags) (
      lib.attrValues (
        lib.filterAttrs (_: machine: machine.config.${namespace}.tailscale.trusted) machines
      )
    )
  );
in
{
  inherit
    services
    devices
    trustedTags
    sharedPorts
    ;
  tags = sortStrings (
    lib.concatMap (svc: svc.tags) (lib.attrValues services)
    ++ lib.concatMap (device: device.tags) (lib.attrValues devices)
  );
}
