{ pkgs, lib, ... }:
{
  _module.args.mkClanSecret = name: {
    program = [
      (lib.getExe (
        pkgs.writeShellApplication {
          inherit name;
          runtimeInputs = [ pkgs.jq ];
          text = ''
            secret=$(clan secrets get tf-${name})
            if [ -z "$secret" ]; then
              echo "clan secret tf-${name} is missing or empty" >&2
              exit 1
            fi
            jq -n --arg secret "$secret" '{"secret":$secret}'
          '';
        }
      ))
    ];
  };

  variable.passphrase = { };

  terraform.required_providers.external.source = "hashicorp/external";
}
