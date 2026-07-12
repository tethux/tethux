{ lib, ... }:

{
  boot.kernel.sysctl = {
    "net.ipv4.ip_forward" = 1;
    "net.ipv6.conf.all.forwarding" = 1;
    "net.bridge.bridge-nf-call-iptables" = 1;
    "net.bridge.bridge-nf-call-ip6tables" = 1;
  };

  networking.nftables.enable = lib.mkDefault true;

  networking.firewall.allowedUDPPorts = [ 24000 ];

  systemd.tmpfiles.rules = [
    "d /var/lib/tethux-ci 0755 ci ci -"
    "d /var/cache/tethux-ci 0755 ci ci -"
    "d /var/log/tethux-ci 0755 ci ci -"
  ];
}
