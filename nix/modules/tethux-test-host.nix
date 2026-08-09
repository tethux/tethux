{ config, lib, ... }:

{
  options.tethux.testHost.hostLabels = lib.mkOption {
    type = lib.types.attrsOf lib.types.str;
    default = { };
    description = "Labels documented for the external Woodpecker runner registration.";
  };

  config = {
    # The test framework uses one explicit marker instead of guessing from
    # provider-specific CI environment variables.
    environment.variables.TETHUX_CI_RUNNER = "1";

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
