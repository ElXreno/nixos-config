{ lib, tailnet, ... }:

let
  dmzTag = "tag:dmz";
  credentialTag = "tag:terraform";
  owner = "ElXreno@github";

  trusted = tailnet.trustedTags;

  policy = {
    tagOwners =
      lib.genAttrs tailnet.tags (_: [
        "autogroup:admin"
        credentialTag
      ])
      // {
        ${credentialTag} = [ "autogroup:admin" ];
      };

    nodeAttrs = [
      {
        target = [ "autogroup:member" ];
        attr = [ "funnel" ];
      }
    ];

    autoApprovers.services = lib.mapAttrs' (
      name: svc: lib.nameValuePair "svc:${name}" svc.tags
    ) tailnet.services;

    grants = [
      {
        src = trusted ++ [ "autogroup:member" ];
        dst = [ "*" ];
        ip = [ "*" ];
      }
      {
        src = [
          "tag:workstation"
          owner
        ];
        dst = tailnet.services.grafana.tags;
        app."elxreno.com/cap/grafana" = [
          {
            login = "admin";
            role = "Admin";
          }
        ];
      }
    ]
    ++ lib.optionals (tailnet.services ? roj) [
      {
        src = [
          "tag:workstation"
          owner
        ];
        dst = tailnet.services.roj.tags;
        app."elxreno.com/cap/roj" = [ { review = true; } ];
      }
    ]
    ++ map (shared: {
      src = [ "autogroup:shared" ];
      dst = shared.tags;
      ip = shared.ports;
    }) tailnet.sharedPorts;

    tests = [
      {
        src = dmzTag;
        deny = [
          "tag:server:22"
          "tag:workstation:22"
          "tag:laptop:22"
          "tag:hass:443"
        ];
      }
      {
        src = "tag:workstation";
        accept = [
          "${dmzTag}:22"
          "${dmzTag}:8428"
          "${dmzTag}:9428"
          "tag:server:22"
        ];
      }
      {
        src = "tag:server";
        accept = [
          "${dmzTag}:8428"
          "${dmzTag}:9428"
        ];
      }
      {
        src = owner;
        accept = [
          "tag:server:22"
          "${dmzTag}:22"
        ];
      }
    ];
  };
in
{
  resource.tailscale_acl.this.acl = builtins.toJSON policy;
}
