{
  image.label = "NIX";

  deploy.host = "10.10.20.1";

  system.keepGenerations = 5;

  services.tailscale.enable = true;

  services.dropbear = {
    enable = true;
    authorizedKeys = with import ../../inventory/_ssh-keys.nix; [
      desktop
      gmail
      yubikey
    ];
  };
}
