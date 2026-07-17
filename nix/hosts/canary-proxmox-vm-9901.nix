{ ... }:

{
  networking.hostName = "tethux-canary-proxmox-vm-9901";

  # This is a KVM guest on a remotely operated Proxmox host. Keep the fleet
  # workload to container providers and network backends; nested hypervisors
  # are deliberately not part of this runner's contract.
  systemd.tpm2.enable = false;
  boot.initrd.systemd.tpm2.enable = false;
  boot.initrd.availableKernelModules = [
    "sd_mod"
    "virtio_blk"
    "virtio_net"
    "virtio_pci"
    "virtio_scsi"
  ];
  services.qemuGuest.enable = true;
  services.tailscale.enable = true;

  tethux.canary = {
    enableVirtualBox = false;
    enableVmware = false;
    enableNestedHypervisors = false;
    hostLabels = {
      "tethux-canary" = "true";
      "linux" = "true";
      "privileged" = "true";
      "vm" = "true";
      "optional" = "true";
      "hypervisor" = "proxmox";
      "host" = "canary-proxmox-vm-9901";
      "device-id" = "proxmox-vm-9901";
      "bootstrap-ip" = "192.168.0.107";
    };
  };
}
