{ config, lib, pkgs, ... }:

let
  cfg = config.tethux.testHost;
  alpineCloudImage = pkgs.fetchurl {
    url = "https://dl-cdn.alpinelinux.org/alpine/v3.24/releases/cloud/generic_alpine-3.24.1-x86_64-bios-tiny-r0.qcow2";
    hash = "sha256-VSXg8d3gVnBunddE9Fq/qUTMTxDkSSL+KinmkFZ5Dso=";
  };
in
{
  options.tethux.testHost = {
    enableVirtualBox = lib.mkEnableOption "VirtualBox support for bare-metal test hosts";
    enableVmware = lib.mkEnableOption "experimental VMware tooling for bare-metal test hosts";
    enableNestedHypervisors = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Enable QEMU/KVM and libvirt for privileged hypervisor smoke tests.";
    };
  };

  config = {
    environment.variables.TETHUX_LIBVIRT_IMAGE = alpineCloudImage;

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
        xorriso
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
