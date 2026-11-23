{ inputs, clanMeta, ... }:
{
  perSystem =
    { config, pkgs, ... }:
    let
      inherit (pkgs) lib;

      dns = import ../terraform/lib/dns.nix {
        inherit lib;
        namespace = clanMeta.name;
        inherit (inputs.self.clan) nixosConfigurations;
      };

      workers = lib.filterAttrs (_: type: type == "directory") (builtins.readDir ../cf-workers);
    in
    {
      checks = lib.mapAttrs' (
        name: _:
        lib.nameValuePair "cf-worker-${name}" (
          pkgs.runCommand "cf-worker-${name}"
            {
              nativeBuildInputs = [
                pkgs.nodejs
                pkgs.wrangler
              ];
              env.WRANGLER_SEND_METRICS = "false";
            }
            ''
              cp -r ${../cf-workers + "/${name}"} worker
              chmod -R u+w worker
              cd worker
              export HOME=$TMPDIR
              node --test
              wrangler deploy --dry-run --outdir $out
            ''
        )
      ) workers;

      packages.deploy-cf-workers = pkgs.writeShellApplication {
        name = "deploy-cf-workers";
        runtimeInputs = [
          config.packages.clan-cli
          pkgs.curl
          pkgs.gitMinimal
          pkgs.jq
          pkgs.wrangler
        ];
        runtimeEnv = {
          CF_ZONE = dns.zone;
          WRANGLER_SEND_METRICS = "false";
        };
        text = builtins.readFile ./deploy-cf-workers.sh;
      };
    };
}
