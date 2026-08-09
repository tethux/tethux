{ config, lib, ... }:

{
  options.tethux.testHost.hostLabels = lib.mkOption {
    type = lib.types.attrsOf lib.types.str;
    default = { };
    description = "Labels documented for the external Woodpecker runner registration.";
  };

  config = {
    tethux.testHost.hostLabels = lib.optionalAttrs config.tethux.testHost.enableNestedHypervisors {
      "hypervisor-libvirt" = "true";
      kvm = "true";
    };

    environment.etc."tethux/test-host-labels".text =
      lib.concatStringsSep "\n" (
        lib.mapAttrsToList (name: value: "${name}=${value}") config.tethux.testHost.hostLabels
      )
      + "\n";
  };
}
