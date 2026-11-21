{ lib, tailnet, ... }:

let
  resourceName = lib.toLower;
in
{
  data.tailscale_device = lib.mapAttrs' (
    hostname: _: lib.nameValuePair (resourceName hostname) { inherit hostname; }
  ) tailnet.devices;

  resource.tailscale_device_tags = lib.mapAttrs' (
    hostname: device:
    lib.nameValuePair (resourceName hostname) {
      device_id = "\${data.tailscale_device.${resourceName hostname}.node_id}";
      inherit (device) tags;
      depends_on = [ "tailscale_acl.this" ];
    }
  ) tailnet.devices;

  resource.tailscale_tailnet_settings.this = {
    acls_externally_managed_on = true;
    acls_external_link = "https://github.com/ElXreno/nixos-config/blob/main/terraform/tailscale-policy.nix";
    https_enabled = true;
    devices_approval_on = true;
    devices_auto_updates_on = false;
    devices_key_duration_days = 180;
    users_approval_on = true;
    users_role_allowed_to_join_external_tailnet = "admin";
    network_flow_logging_on = false;
    regional_routing_on = false;
    posture_identity_collection_on = false;
  };

  resource.tailscale_dns_configuration.this = {
    magic_dns = true;
    override_local_dns = false;
    nameservers = [ ];
    search_paths = [ ];
    split_dns = [ ];
  };
}
