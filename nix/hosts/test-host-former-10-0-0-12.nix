{ lib, ... }:

{
  networking.hostName = "tethux-test-host-former-10-0-0-12";

  tethux.testHost = {
    enableVirtualBox = true;
    enableVmware = false;
    enableNestedHypervisors = true;
    hostLabels = {
      "tethux-test-host" = "true";
      "linux" = "true";
      "privileged" = "true";
      "baremetal" = "true";
      "host" = "test-host-former-10-0-0-12";
      "old-ip" = "10.0.0.12";
      "current-ip" = "10.0.0.78";
    };
  };

  networking.interfaces = lib.mkDefault { };
}
