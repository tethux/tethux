{ pkgs, lib, ... }:

{
  virtualisation = {
    containers.registries.insecure = [ "127.0.0.1:5000" ];

    docker = {
      enable = true;
      autoPrune.enable = true;
    };

    podman = {
      enable = true;
      dockerSocket.enable = false;
      defaultNetwork.settings.dns_enabled = true;
    };

    containerd.enable = true;
  };

  environment.systemPackages = with pkgs; [
    containerd
    cni-plugins
    docker-client
    docker-compose
    podman
    runc
    slirp4netns
  ];

  environment.variables = {
    DOCKER_HOST = lib.mkDefault "unix:///var/run/docker.sock";
    CONTAINER_HOST = lib.mkDefault "unix:///run/podman/podman.sock";
    CONTAINERD_ADDRESS = lib.mkDefault "/run/containerd/containerd.sock";
  };
}
