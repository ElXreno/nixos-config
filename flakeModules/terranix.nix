_: {
  perSystem =
    { pkgs, inputs', ... }:
    let
      # https://github.com/tailscale/terraform-provider-tailscale
      # TODO: Drop when tailscale_service resource lands in a released version
      tailscaleProvider = pkgs.terraform-providers.tailscale_tailscale.override {
        rev = "d8848da4e737e49138683d4feaba6259d75b0de0";
        version = "0.29.0";
        hash = "sha256-XITwEn9XdRXkVvOiAsoHm2Lhj5/JI0N52Njl4Lgs+V8=";
        vendorHash = "sha256-nSxQYOvAF4FeLX1Qz8dT24F72Zm7dtskfcFg68dq/c4=";
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
