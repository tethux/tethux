{ lib, ... }:

{
  networking.hostName = "tethux-canary-10-0-0-11";

  tethux.canary = {
    enableVirtualBox = true;
    enableVmware = false;
    enableNestedHypervisors = true;
    hostLabels = {
      "tethux-canary" = "true";
      "linux" = "true";
      "privileged" = "true";
      "baremetal" = "true";
      "host" = "canary-10-0-0-11";
      "expected-ip" = "10.0.0.11";
    };
  };

  networking.interfaces = lib.mkDefault { };
}
