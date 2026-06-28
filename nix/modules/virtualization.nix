{ config, lib, pkgs, ... }:

let
  cfg = config.tethux.canary;
in
{
  options.tethux.canary = {
    enableVirtualBox = lib.mkEnableOption "VirtualBox host support for bare-metal canaries";
    enableVmware = lib.mkEnableOption "experimental VMware tooling for bare-metal canaries";
    enableNestedHypervisors = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Enable QEMU/KVM and libvirt for privileged hypervisor smoke tests.";
    };
  };

  config = {
    virtualisation.libvirtd = lib.mkIf cfg.enableNestedHypervisors {
      enable = true;
      qemu = {
        package = pkgs.qemu_kvm;
        runAsRoot = false;
        swtpm.enable = true;
      };
    };

    virtualisation.virtualbox.host.enable = lib.mkIf cfg.enableVirtualBox true;

    environment.systemPackages =
      (with pkgs; [
        dnsmasq
        dynamips
        libvirt
        OVMF
        qemu_kvm
        swtpm
        virtiofsd
      ])
      ++ lib.optionals cfg.enableVirtualBox (with pkgs; [
        virtualbox
      ])
      ++ lib.optionals cfg.enableVmware (with pkgs; [
        open-vm-tools
        ovftool
      ]);
  };
}
