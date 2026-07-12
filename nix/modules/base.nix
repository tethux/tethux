{
  pkgs,
  lib,
  self ? null,
  ...
}:

let
  tethuxPackage =
    if self == null then
      null
    else
      self.packages.${pkgs.stdenv.hostPlatform.system}.tethux or null;
in

{
  nixpkgs.config.allowUnfree = true;

  hardware.enableRedistributableFirmware = true;

  nix = {
    settings = {
      experimental-features = [
        "nix-command"
        "flakes"
      ];
      trusted-users = [
        "root"
        "veya"
        "ci"
      ];
    };
  };

  boot.tmp.cleanOnBoot = true;
  boot.kernelModules = [
    "tun"
    "tap"
    "dummy"
    "bridge"
    "br_netfilter"
  ];
  boot.loader = {
    systemd-boot.enable = lib.mkDefault true;
    efi.canTouchEfiVariables = lib.mkDefault false;
  };

  fileSystems = {
    "/" = {
      # These names are created by nix/installers/disko-laptop.nix. Using
      # filesystem labels here broke live configuration switches because the
      # installed ESP/root filesystems intentionally have no filesystem label.
      device = lib.mkDefault "/dev/disk/by-partlabel/disk-main-root";
      fsType = lib.mkDefault "ext4";
    };
    "/boot" = {
      device = lib.mkDefault "/dev/disk/by-partlabel/disk-main-ESP";
      fsType = lib.mkDefault "vfat";
      options = lib.mkDefault [
        "nofail"
        "x-systemd.device-timeout=10s"
      ];
    };
  };

  services.openssh = {
    enable = true;
    settings = {
      PasswordAuthentication = false;
      KbdInteractiveAuthentication = false;
      PermitRootLogin = lib.mkDefault "prohibit-password";
    };
  };

  services.logind.settings.Login = {
    HandleLidSwitch = "ignore";
    HandleLidSwitchDocked = "ignore";
    HandleLidSwitchExternalPower = "ignore";
  };

  networking = {
    networkmanager.enable = lib.mkDefault true;
    firewall = {
      enable = true;
      allowedTCPPorts = [
        22
      ];
    };
  };

  environment.systemPackages =
    (with pkgs; [
      bashInteractive
      bridge-utils
      curl
      ethtool
      git
      go
      golangci-lint
      gofumpt
      gotools
      iproute2
      iptables
      jq
      libpcap
      mise
      nmap
      pciutils
      pkg-config
      socat
      tcpdump
      tmux
      usbutils
    ])
    ++ lib.optional (tethuxPackage != null) tethuxPackage;

  security.sudo.enable = true;
  time.timeZone = lib.mkDefault "Europe/Vienna";
  system.stateVersion = lib.mkDefault "25.11";
}
