{ lib, ... }:

let
  sshKeys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII8bd7rq0jO6ZvT8jKLZrur3xu1u1l5oe9K+k5XM5tJ/ lolspamdisemail@gmail.com"
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICKZ7lg6/lehc7oAnkCTde1+JNiR15fgnu/tgW+H4xpb 0xveya@gmail.com"
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDT3+iRJYcXuH88OHW+lnprEZ4kUPDLg+gVyjC/f6QQ+ veya@windows12"
  ];
in
{
  users.users.veya = {
    isNormalUser = true;
    extraGroups = [
      "wheel"
      "networkmanager"
      "docker"
      "podman"
      "kvm"
      "libvirtd"
      "vboxusers"
    ];
    openssh.authorizedKeys.keys = sshKeys;
  };

  users.users.ci = {
    isNormalUser = true;
    description = "tethux canary test runner";
    extraGroups = [
      "wheel"
      "docker"
      "podman"
      "kvm"
      "libvirtd"
      "vboxusers"
    ];
    openssh.authorizedKeys.keys = sshKeys;
  };

  security.sudo.extraRules = [
    {
      users = [
        "ci"
        "veya"
      ];
      commands = [
        {
          command = "ALL";
          options = [ "NOPASSWD" ];
        }
      ];
    }
  ];

  assertions = [
    {
      assertion = lib.isList sshKeys;
      message = "sshKeys must remain a list of public SSH keys.";
    }
  ];
}
