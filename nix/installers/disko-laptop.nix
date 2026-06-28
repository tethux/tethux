{ lib, ... }:

let
  disk = builtins.getEnv "TETHUX_INSTALL_DISK";
in
{
  assertions = [
    {
      assertion = disk != "";
      message = "Refusing to define destructive disk layout without TETHUX_INSTALL_DISK=/dev/...";
    }
  ];

  disko.devices = {
    disk.main = {
      type = "disk";
      device = disk;
      content = {
        type = "gpt";
        partitions = {
          ESP = {
            size = "1G";
            type = "EF00";
            content = {
              type = "filesystem";
              format = "vfat";
              mountpoint = "/boot";
            };
          };
          root = {
            size = "100%";
            content = {
              type = "filesystem";
              format = "ext4";
              mountpoint = "/";
            };
          };
        };
      };
    };
  };
}
