{ config, lib, pkgs, self, ... }:

let
  cfg = config.tethux.testHost;
  alpineCloudImage = self.packages.${pkgs.system}.libvirtFixture;
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

    networking.firewall.extraReversePathFilterRules = lib.mkIf cfg.enableNestedHypervisors ''
      iifname "tx*" accept comment "tethux libvirt test bridges"
    '';

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
