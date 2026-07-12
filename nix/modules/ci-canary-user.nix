{ lib, ... }:

let
  sshKeys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII8bd7rq0jO6ZvT8jKLZrur3xu1u1l5oe9K+k5XM5tJ/ lolspamdisemail@gmail.com"
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICKZ7lg6/lehc7oAnkCTde1+JNiR15fgnu/tgW+H4xpb 0xveya@gmail.com"
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDT3+iRJYcXuH88OHW+lnprEZ4kUPDLg+gVyjC/f6QQ+ veya@windows12"
    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDdfTbrpsBBbGia/BIvychIQzSjBvZhgYvZwk+eY7kcorPQ3kd3Nci83o9U1iC9Bt83ZqdH0KoWTdF9V9yqvxNVOBWLwHz8TULClwo9kZvrplpu3g1qBAoYAYPW9hZONwWW08k2PvkKq4DBY7HVq7T7rRu+2el7OEfCL9n9RaVeTCOti7suWF/Ju9AXvklgVI49NxAKRUSKoPBspdMBpyrG+THVwtLhBBTpN2/dA053tiKP4cSxtBgF28yXAZUXshTgDt0a4229lrg56B78s/41mXADDB+id8gzFwlN1YzY8IJkyPICNGDb0JzeHOOW7DWMg+wq+BE5qS9bOXSpvDD6xCh6zkqiYlU3Rh22bxHGqnt8rVtLjM0xcpTMFxjzR5MI4ZOljt3Z+7/5jPQd8o3FwtaaSbvMpJSgCF3P2snCO4qZMuW7Ci1MT2dwAsP8mVJXikLDndwBy94zaUdpy4hGg1ODdvudzysfCf02ztzjuYM3Do/xY2zuLVO6Xq1TggreIJzX+gFkmmgHAamfRq5NUbvjN545Pw6dr20oQr/9GiIDHu+xgOAuOSmTCy4ZZKOuz55oYRubYJN4KZ0exOX67026cqrkpJidSq4ABvTu5Gc9HD7npbnqVa+z3h1oNVG1hyTfp9Oh/FhtYq5svJq3mj5LQy/RGmmvTWAGDw0Fdw== root@nas"
  ];
in
{
  users.groups.ci = { };

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
    group = "ci";
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
