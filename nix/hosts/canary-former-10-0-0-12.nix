{ lib, ... }:

{
  networking.hostName = "tethux-canary-former-10-0-0-12";

  tethux.canary = {
    enableVirtualBox = true;
    enableVmware = false;
    enableNestedHypervisors = true;
    hostLabels = {
      "tethux-canary" = "true";
      "linux" = "true";
      "privileged" = "true";
      "baremetal" = "true";
      "host" = "canary-former-10-0-0-12";
      "old-ip" = "10.0.0.12";
      "current-ip" = "10.0.0.78";
    };
  };

  networking.interfaces = lib.mkDefault { };
}
