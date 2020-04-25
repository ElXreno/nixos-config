{ config, pkgs, ... }:
let
  unstable = import <nixos-unstable> {
    overlays = [ (self: super: {
      chromium = super.chromium.override { enableVaapi = true; };
    }) ];
    config = removeAttrs config.nixpkgs.config [ "packageOverrides" ];
  };
in
{
  nixpkgs.config.allowUnfree = true;

  boot.kernelPackages = unstable.linuxPackages_latest;
  boot.kernelParams = [ "mitigations=off" "scsi_mod.use_blk_mq=1" ];
  boot.kernel.sysctl = {
    "kernel.sysrq" = 1;
    "vm.swappiness" = 100;
    "vm.dirty_expire_centisecs" = 1000;
    "vm.dirty_writeback_centisecs" = 300;
    "net.ipv4.tcp_fastopen" = 3;
  };

  boot.loader.grub = {
    enable = true;
    version = 2;
    device = "/dev/disk/by-id/wwn-0x5002538e700e7485";
    useOSProber = true; # Too slow
  };

  networking = {
    hostName = "desktop";
    networkmanager.enable = true;
    firewall = {
      allowedTCPPorts = [ 22 22000 ];
      allowedUDPPorts = [ 21027 ];
    };
  };

  console = {
    packages = [ pkgs.terminus_font ];
    font = "ter-i16n";
    keyMap = "us";
  };

  i18n.defaultLocale = "ru_RU.UTF-8";
  time.timeZone = "Europe/Minsk";
  
  sound.enable = true;
  hardware.pulseaudio = {
    enable = true;
    package = pkgs.pulseaudioFull;
  };
  
  users.users.elxreno = {
    isNormalUser = true;
    extraGroups = [ "wheel" "networkmanager" ];
    initialPassword = "password";
  };
  
  home-manager.users.elxreno = {
    services.syncthing.enable = true;
    
    home = {
      packages = with pkgs; [
        wget kakoune htop iotop
        kate
      ] ++ (with unstable; [
        chromium
        firefox
        tdesktop
      ]);
      sessionVariables = {
        EDITOR = "kakoune";
      };
    };
    
    nixpkgs.overlays = [
      (self: super: {
        syncthing = unstable.syncthing;
      })
    ];
  };
  
#   environment.systemPackages = with pkgs; [
#     wget vim firefox htop kate
#   ] ++ (with unstable; [
#     chromium
#     tdesktop
#   ]);
  
  programs = {
    #mtr.enable = true;
    #gnupg.agent = {
    #  enable = true;
    #  enableSSHSupport = true;
    #  pinentryFlavor = "gnome3";
    #};
    ccache.enable = true;
  };

  services = {
    openssh.enable = true;
    xserver = {
      enable = true;
      layout = "us,ru";
      xkbOptions = "grp:alt_shift_toggle";
      displayManager.sddm.enable = true;
      desktopManager.plasma5.enable = true;
      videoDrivers = [ "nvidia" ];
    };
    #printing.enable = true;
    
    udev.extraRules = ''
      ACTION=="add|change", KERNEL=="sd[a-z]", ATTR{queue/rotational}=="1", ATTR{queue/scheduler}="bfq"
    '';
  };
  
  system.autoUpgrade = {
    enable = true;
    dates = "9:00";
  };

}

