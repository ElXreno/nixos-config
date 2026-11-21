{ inputs, clanMeta, ... }:
{
  perSystem =
    { config, pkgs, ... }:
    let
      terraformPackage = pkgs.opentofu.withPlugins (p: [
        p.tailscale_tailscale
        p.cloudflare_cloudflare
        p.hashicorp_aws
        p.hashicorp_external
      ]);

      tailnet = import ../terraform/lib/tailnet.nix {
        inherit (pkgs) lib;
        namespace = clanMeta.name;
        inherit (inputs.self.clan) nixosConfigurations;
      };

      s4 = import ../terraform/lib/s4.nix {
        inherit (pkgs) lib;
        namespace = clanMeta.name;
        inherit (inputs.self.clan) nixosConfigurations;
      };

      dns = import ../terraform/lib/dns.nix {
        inherit (pkgs) lib;
        namespace = clanMeta.name;
        inherit (inputs.self.clan) nixosConfigurations;
      };

      tunnels = import ../terraform/lib/tunnels.nix {
        inherit (pkgs) lib;
        namespace = clanMeta.name;
        inherit (inputs.self.clan) nixosConfigurations;
      };
    in
    {
      terranix.terranixConfigurations.terraform = {
        workdir = "terraform";
        modules = [
          ../terraform/common.nix
          ../terraform/tailscale-services.nix
          ../terraform/tailscale-policy.nix
          ../terraform/tailscale-tailnet.nix
          ../terraform/s4-policy.nix
          ../terraform/cloudflare-dns.nix
          ../terraform/cloudflare-zone.nix
          ../terraform/cloudflare-workers.nix
          ../terraform/cloudflare-tunnels.nix
        ];
        extraArgs = {
          inherit
            tailnet
            s4
            dns
            tunnels
            ;
        };
        terraformWrapper.package = terraformPackage;
        terraformWrapper.extraRuntimeInputs = [ config.packages.clan-cli ];
        terraformWrapper.prefixText = ''
          TF_VAR_passphrase=$(clan secrets get tf-passphrase)
          export TF_VAR_passphrase
          TF_ENCRYPTION=$(cat <<'EOF'
          key_provider "pbkdf2" "state_encryption_password" {
            passphrase = var.passphrase
          }
          method "aes_gcm" "encryption_method" {
            keys = key_provider.pbkdf2.state_encryption_password
          }
          state {
            enforced = true
            method = method.aes_gcm.encryption_method
          }
          EOF
          )

          export TF_ENCRYPTION

          if [ -e config.tf.json ]; then
            rm -f .terraform.lock.hcl
            ${terraformPackage}/bin/tofu providers lock \
              -fs-mirror="${terraformPackage}/libexec/terraform-providers" \
              -platform=${pkgs.go.GOOS}_${pkgs.go.GOARCH} >/dev/null
          fi
        '';
      };
    };
}
