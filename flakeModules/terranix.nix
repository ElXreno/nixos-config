_: {
  perSystem =
    { pkgs, inputs', ... }:
    let
      # https://github.com/tailscale/terraform-provider-tailscale
      # TODO: Drop when tailscale_service resource lands in a released version
      tailscaleProvider = pkgs.terraform-providers.tailscale_tailscale.override {
        rev = "957d72053ef841e87ab0a17754f5d107e97f52db";
        version = "0.29.0";
        hash = "sha256-nKJXwmFKrivWHOCga8o3fYstmr+fYs1uaZ+YpRTk+AM=";
        vendorHash = "sha256-DkIl2KcDlztjKoMWPdV4WzTpraVJgfXftyM9IIGNTxE=";
      };

      terraformPackage = pkgs.opentofu.withPlugins (p: [
        tailscaleProvider
        p.hashicorp_external
      ]);
    in
    {
      terranix.terranixConfigurations.terraform = {
        workdir = "terraform";
        modules = [ ../tofu/tailscale-services.nix ];
        terraformWrapper.package = terraformPackage;
        terraformWrapper.extraRuntimeInputs = [ inputs'.clan-core.packages.clan-cli ];
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
        '';
      };
    };
}
