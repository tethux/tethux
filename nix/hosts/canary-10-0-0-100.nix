{ lib, ... }:

{
  networking.hostName = "tethux-canary-10-0-0-100";

  # This laptop has no TPM device. Disable systemd's upstream TPM units so
  # live switches and boots do not wait for dev-tpm0/dev-tpmrm0 twice.
  systemd.tpm2.enable = false;
  boot.initrd.systemd.tpm2.enable = false;

  tethux.canary = {
    enableVirtualBox = true;
    enableVmware = false;
    enableNestedHypervisors = true;
    hostLabels = {
      "tethux-canary" = "true";
      "linux" = "true";
      "privileged" = "true";
      "baremetal" = "true";
      "host" = "canary-10-0-0-100";
      "current-ip" = "10.0.0.100";
    };
  };

  networking.interfaces = lib.mkDefault { };
}
