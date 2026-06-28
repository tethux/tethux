{ config, lib, ... }:

{
  options.tethux.canary.hostLabels = lib.mkOption {
    type = lib.types.attrsOf lib.types.str;
    default = { };
    description = "Labels documented for the external Woodpecker runner registration.";
  };

  config = {
    environment.etc."tethux/canary-labels".text =
      lib.concatStringsSep "\n" (
        lib.mapAttrsToList (name: value: "${name}=${value}") config.tethux.canary.hostLabels
      )
      + "\n";
  };
}
